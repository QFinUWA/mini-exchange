#!/usr/bin/env bash
# Build both images locally and (re)deploy them to the QFin VPS.
#   deploy/deploy.sh            # build + ship + restart
# VPS: ssh target for the QFin VPS (needs your key in root's authorized_keys).
# JUMP: optional machine to hop through if the VPS key lives somewhere else, e.g. JUMP=mac.
set -euo pipefail
cd "$(dirname "$0")/.."
JUMP=${JUMP-}
VPS=${VPS:-root@66.226.147.134}
REMOTE_DIR=/opt/mini-exchange

# remote "<command>": run a shell command on the VPS (quoted so pipes run there, not on the Mac)
remote() {
  if [ -n "$JUMP" ]; then ssh "$JUMP" "ssh -o BatchMode=yes $VPS $(printf '%q' "$1")"
  else ssh -o BatchMode=yes "$VPS" "$1"; fi
}

docker build -f deploy/Dockerfile.server -t qfin-mini-exchange-server:latest .
docker build -f deploy/Dockerfile.web    -t qfin-mini-exchange-web:latest .
docker build -f deploy/Dockerfile.runner -t qfin-mini-exchange-runner:latest .

echo "shipping images..."
docker save qfin-mini-exchange-server:latest qfin-mini-exchange-web:latest qfin-mini-exchange-runner:latest | gzip -1 | remote "gunzip | docker load"
remote "mkdir -p $REMOTE_DIR && cat > $REMOTE_DIR/docker-compose.yml" < deploy/docker-compose.yml
remote "cd $REMOTE_DIR && docker compose up -d && docker image prune -f >/dev/null && docker compose ps"
