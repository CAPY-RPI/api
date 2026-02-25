# INIT Migration Runbook

1. stop auto deploy service
2. stop api service
3. pull latest image
4. run one off migration command: `docker run --rm --env-file .env -e FORCE=1 <registry>/<image>:<tag> <migrate-command>`
5. run `docker compose up -d` as usual
6. restart auto deploy service
