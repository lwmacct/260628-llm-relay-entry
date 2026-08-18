# CI Integration Driver

This directory contains the GitHub Actions integration-test driver for the
published `latest` relay images. It is CI-only and is not a local development
or deployment command.

The workflow in `.github/workflows/integration.yml` invokes `run.sh` on an
amd64 GitHub runner. The driver creates one isolated PostgreSQL service with
`console` and `vendor` databases using the CI-only `ci`/`ci` credential, then
creates the Docker network, seeds Vendor and Console,
then actively calls the Entry HTTP endpoint through directive-proxy.

Do not point this Compose project at production configuration, credentials, or
databases. Changes to the test topology should be made together with the
workflow and its documentation.
