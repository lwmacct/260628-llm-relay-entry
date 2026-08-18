# Latest Image Integration

Entry is the source of the public request, so its integration driver starts a
temporary relay topology and actively calls Entry. The test uses the published
`latest` image for Entry, Console, Vendor and directive-proxy; it does not build
source code or read production deployment files.

The GitHub Actions workflow invokes the driver on an amd64 runner. The driver
pulls the four images, starts one isolated PostgreSQL service with `console` and
`vendor` databases using the CI-only `ci`/`ci` credential, creates the Docker
network, seeds Vendor, passes Vendor's complete RemoteSpec to Console, seeds a
Console API key, then starts the runtime services. It waits for Vendor
`/api/health`, directive-proxy `/health`, and Entry `/readyz` before sending the
request sequence:

- valid API Token returns 200 and reaches the httpbingo upstream;
- invalid API Token returns 401;
- forged `X-Relay-Route-ID` and `X-Resolver-Affinity-Key` headers do not affect
  routing or appear in the upstream response.

The GitHub Actions workflow is triggered manually, nightly, or by a
`relay-latest-published` repository dispatch event. All credentials are
temporary CI values. Resolver and API tokens are captured only inside the
single test job, masked in GitHub logs, and never uploaded as artifacts.

The deployment scripts under `/data/project/deploy/2310-llm-relay` remain
production-only. This integration driver follows their service topology but
uses generated CI configuration and an isolated PostgreSQL volume.

The files under `.github/integration/` are CI-only. Do not run the driver
against a local or production environment.
