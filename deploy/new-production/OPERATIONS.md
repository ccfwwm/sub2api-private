# Migrated production deployment

The production target is `206.82.2.46:19565`. Run **Manual Production Deployment** from `main`, with `verify_only=true` to check connectivity and the current hardened application without releasing a version. To deploy, select a published stable `vMAJOR.MINOR.PATCH` release and leave `verify_only=false`.

Repository and `production` environment settings use `DEPLOY_HOST`, `DEPLOY_PORT`, `DEPLOY_USER`, `DEPLOY_SSH_KEY` and `DEPLOY_KNOWN_HOSTS`. Each repository has its own forced-command key. Keep strict host-key verification enabled. Never reuse a key from the retired host.

The root-owned gateway at `/opt/codeme-deploy/sub2api` validates the upload, checks the release revision and immutable migration inventory, rebuilds and scans the runtime, backs up PostgreSQL from one exported snapshot and verifies restoration on an isolated database. It waits for an idle window and drains active requests before replacing only application and documentation containers. Database containers, configuration, persistent mounts, Nginx and firewall stay host-owned. Failed application deployment does not restore a backup over live writes.

The database migration inventory must exactly match the release. A release containing new schema changes requires a separately reviewed migration procedure; this ordinary deployment refuses it before application cutover. Automatic downgrade and forced interruption are disabled.

Durable jobs and private backup/scan receipts are under `/var/lib/codeme-deploy/sub2api`. An interrupted Actions runner does not stop the host job. Inspect the existing job before retrying. The gateway and trusted Dockerfiles in this directory are host installation sources; a normal source upload cannot replace the installed gateway.
