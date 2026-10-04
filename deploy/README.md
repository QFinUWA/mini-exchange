# Deploying to the QFin VPS

Live at https://exchange.qfinuwa.org (VPS `qfin-new`, 66.226.147.134).

- `/opt/mini-exchange/docker-compose.yml` runs `exchange-server` (Go, state + Parquet data in the
  `mini-exchange_exchange-data` volume) and `exchange-web` (Next.js) on the `infra_web` network.
- Caddy (in `/opt/infra`) routes `exchange.qfinuwa.org`: `/api/*` and `/ws` to the server, the rest
  to the web app. The block is in `Caddyfile.snippet`.
- Images are built locally because the VPS (1 CPU, 1 GB RAM) can't build Next.js.

Redeploy after code changes:

    deploy/deploy.sh

It builds both images (needs Docker locally), streams them to the VPS over ssh, and restarts the
containers. Exchange state survives redeploys (it lives in the volume). Your ssh key must be in
the VPS's `/root/.ssh/authorized_keys`; set `VPS=<host alias>` if you use an ssh config alias, or
`JUMP=<host>` to hop through another machine that has the key.

The admin login is handed over separately (never commit it).

Backup the exchange state:

    ssh root@66.226.147.134 'docker run --rm -v mini-exchange_exchange-data:/d alpine tar cz -C /d .' > exchange-backup.tgz
