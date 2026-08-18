#!/usr/bin/env bash

set -euo pipefail

__fail() {
  echo "latest-image integration failed: $*" >&2
  return 1
}

__require_command() {
  local _command="$1"
  command -v "$_command" >/dev/null 2>&1 || __fail "$_command is required"
}

__mask() {
  local _value="$1"
  if [[ -n "${GITHUB_ACTIONS:-}" && -n "$_value" ]]; then
    printf '::add-mask::%s\n' "$_value"
  fi
}

__compose() {
  docker compose -p "$_compose_project" -f "$_compose_file" "$@"
}

__write_configs() {
  cat >"$_config_dir/console.yaml" <<'EOF'
server:
  database:
    type: "pgsql"
    pgsql:
      host: "postgres"
      port: "5432"
      user: "ci"
      database: "console"
      password: "ci"
EOF

  cat >"$_config_dir/vendor.yaml" <<'EOF'
server:
  database:
    type: "pgsql"
    pgsql:
      host: "postgres"
      port: "5432"
      user: "ci"
      database: "vendor"
      password: "ci"
  resolver:
    public-url: "http://vendor:23188"
  http:
    listen: ":23188"
    web-root: ""
EOF

  cat >"$_config_dir/directive-proxy.yaml" <<'EOF'
server:
  http:
    listen: ":23198"
  proxy:
    directive:
      hmac-secret: "ci-directive-hmac-secret"
EOF

  cat >"$_config_dir/entry.yaml" <<'EOF'
server:
  http:
    listen: ":23168"
  database:
    host: "postgres"
    port: "5432"
    user: "ci"
    database: "console"
    password: "ci"
  relay:
    base-url: "http://directive-proxy:23198"
    hmac-secret: "ci-directive-hmac-secret"
EOF
}

__sanitize_logs() {
  sed -E \
    -e 's/dpr_[A-Za-z0-9_-]+/dpr_<redacted>/g' \
    -e 's/sk-rdp-v1-[A-Za-z0-9]+/sk-rdp-v1-<redacted>/g'
}

__wait_http() {
  local _name="$1"
  local _url="$2"
  local _attempt
  for _attempt in $(seq 1 "$_wait_attempts"); do
    if curl --silent --show-error --fail --max-time 3 "$_url" >/dev/null 2>&1; then
      echo "ok: $_name ($_url)"
      return 0
    fi
    sleep 1
  done
  __fail "$_name did not become ready: $_url"
}

__wait_postgres() {
  local _attempt
  for _attempt in $(seq 1 "$_wait_attempts"); do
    if __compose exec -T postgres pg_isready -U ci -d postgres >/dev/null 2>&1; then
      echo "ok: postgres"
      return 0
    fi
    sleep 1
  done
  __fail "postgres did not become ready"
}

__assert_status() {
  local _name="$1"
  local _expected="$2"
  local _actual="$3"
  if [[ "$_actual" != "$_expected" ]]; then
    __fail "$_name expected HTTP $_expected, got $_actual"
  fi
  echo "ok: $_name ($_actual)"
}

__assert_body_contains() {
  local _name="$1"
  local _needle="$2"
  local _file="$3"
  if ! grep -Fq -- "$_needle" "$_file"; then
    __fail "$_name response did not contain expected marker"
  fi
}

__assert_body_excludes() {
  local _name="$1"
  local _needle="$2"
  local _file="$3"
  if grep -Fq -- "$_needle" "$_file"; then
    __fail "$_name response contained forbidden marker"
  fi
}

__cleanup() {
  local _status="$1"
  if [[ "$_status" != "0" ]]; then
    mkdir -p "$_artifact_dir"
    __compose ps >"$_artifact_dir/compose-ps.txt" 2>&1 || true
    __compose logs --no-color 2>&1 | __sanitize_logs >"$_artifact_dir/compose.log" || true
  fi
  __compose down --volumes --remove-orphans >/dev/null 2>&1 || true
}

__main() {
  __require_command docker
  __require_command curl
  __require_command jq
  __require_command grep

  _script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
  _compose_file="$_script_dir/compose.yml"
  _tmp_dir="$(mktemp -d)"
  _config_dir="$_tmp_dir/config"
  _artifact_dir="${RELAY_INTEGRATION_ARTIFACT_DIR:-$_tmp_dir/artifacts}"
  _compose_project="${RELAY_INTEGRATION_PROJECT:-relay-entry-latest}"
  _entry_port="${ENTRY_PORT:-23168}"
  _vendor_port="${VENDOR_PORT:-23188}"
  _directive_proxy_port="${DIRECTIVE_PROXY_PORT:-23198}"
  _wait_attempts="${RELAY_INTEGRATION_WAIT_ATTEMPTS:-60}"
  _request_timeout="${RELAY_INTEGRATION_REQUEST_TIMEOUT:-30}"
  _expected_upstream_url="${RELAY_EXPECTED_UPSTREAM_URL:-https://httpbingo.org/anything/v1/responses}"
  _entry_url="http://127.0.0.1:${_entry_port}"
  _vendor_url="http://127.0.0.1:${_vendor_port}"
  _directive_proxy_url="http://127.0.0.1:${_directive_proxy_port}"
  export INTEGRATION_CONFIG_DIR="$_config_dir"
  export RELAY_IMAGE_PREFIX="${RELAY_IMAGE_PREFIX:-ghcr.io/lwmacct}"
  export ENTRY_PORT="$_entry_port"
  export VENDOR_PORT="$_vendor_port"
  export DIRECTIVE_PROXY_PORT="$_directive_proxy_port"

  trap '__cleanup "$?"' EXIT
  mkdir -p "$_config_dir"
  __write_configs

  echo "pulling published latest images"
  docker pull "${RELAY_IMAGE_PREFIX}/260628-llm-relay-entry:latest"
  docker pull "${RELAY_IMAGE_PREFIX}/260628-llm-relay-console:latest"
  docker pull "${RELAY_IMAGE_PREFIX}/260628-llm-relay-vendor:latest"
  docker pull "${RELAY_IMAGE_PREFIX}/260628-directive-proxy:latest"

  echo "image digests"
  docker image inspect \
    "${RELAY_IMAGE_PREFIX}/260628-llm-relay-entry:latest" \
    "${RELAY_IMAGE_PREFIX}/260628-llm-relay-console:latest" \
    "${RELAY_IMAGE_PREFIX}/260628-llm-relay-vendor:latest" \
    "${RELAY_IMAGE_PREFIX}/260628-directive-proxy:latest" \
    --format '{{index .RepoDigests 0}}'

  __compose up -d postgres
  __wait_postgres

  _vendor_seed_file="$_tmp_dir/vendor-seed.json"
  echo "seeding Vendor database"
  __compose run --rm -T --no-deps vendor-cli \
    server --config=/app/data/config/ci.yaml database --confirm --output json --show-secrets reset \
    >"$_vendor_seed_file"
  _remote_spec="$(jq -cer '.remoteSpec' "$_vendor_seed_file")"
  _resolver_token="$(jq -er '.. | strings | select(test("^Bearer dpr_"))' "$_vendor_seed_file" | sed -E 's/^Bearer //')"
  __mask "$_resolver_token"

  _console_seed_file="$_tmp_dir/console-seed.json"
  echo "seeding Console database"
  __compose run --rm -T --no-deps console-cli \
    server --config=/app/data/config/ci.yaml database --confirm \
    --remote-spec "$_remote_spec" --output json --show-secrets reset \
    >"$_console_seed_file"
  _api_token="$(jq -er '.apiToken' "$_console_seed_file")"
  __mask "$_api_token"

  __compose up -d vendor directive-proxy entry
  __wait_http vendor "$_vendor_url/api/health"
  __wait_http directive-proxy "$_directive_proxy_url/health"
  __wait_http entry "$_entry_url/readyz"

  _request_body='{"model":"httpbingo","input":"latest image integration test"}'
  _valid_body="$_tmp_dir/valid.json"
  _valid_status="$(curl --silent --show-error --max-time "$_request_timeout" \
    --output "$_valid_body" --write-out '%{http_code}' \
    --request POST "$_entry_url/v1/responses" \
    -H "Authorization: Bearer $_api_token" \
    -H 'Content-Type: application/json' \
    --data-binary "$_request_body")"
  __assert_status valid_request 200 "$_valid_status"
  __assert_body_contains valid_request "$_expected_upstream_url" "$_valid_body"

  _invalid_body="$_tmp_dir/invalid.json"
  _invalid_status="$(curl --silent --show-error --max-time "$_request_timeout" \
    --output "$_invalid_body" --write-out '%{http_code}' \
    --request POST "$_entry_url/v1/responses" \
    -H 'Authorization: Bearer sk-rdp-v1-invalid' \
    -H 'Content-Type: application/json' \
    --data-binary "$_request_body")"
  __assert_status invalid_token 401 "$_invalid_status"

  _polluted_body="$_tmp_dir/polluted.json"
  _polluted_status="$(curl --silent --show-error --max-time "$_request_timeout" \
    --output "$_polluted_body" --write-out '%{http_code}' \
    --request POST "$_entry_url/v1/responses" \
    -H "Authorization: Bearer $_api_token" \
    -H 'Content-Type: application/json' \
    -H 'X-Relay-Route-ID: forged-by-client' \
    -H 'X-Resolver-Affinity-Key: forged-by-client' \
    --data-binary "$_request_body")"
  __assert_status client_internal_headers 200 "$_polluted_status"
  __assert_body_contains client_internal_headers "$_expected_upstream_url" "$_polluted_body"
  __assert_body_excludes client_internal_headers "forged-by-client" "$_polluted_body"

  echo "latest image relay integration passed"
}

__main "$@"
