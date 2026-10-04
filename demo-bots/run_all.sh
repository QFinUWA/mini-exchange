#!/usr/bin/env bash
# Start the three demo bots against an exchange (default: local server).
# Accounts are created on first login; passwords are generated once and kept in .accounts
set -euo pipefail
cd "$(dirname "$0")"
URL="${EXCHANGE_URL:-http://localhost:3211}"
[ -f .accounts ] || for b in mm-demo arb-demo opt-demo; do echo "$b $(head -c 12 /dev/urandom | base64 | tr -d '/+=')"; done > .accounts
mkdir -p logs
run() { uv run --quiet --with requests --with websockets python -u "$1" -u "$2" -p "$3" --url "$URL" > "logs/$2.log" 2>&1 & echo "started $2 (pid $!) -> logs/$2.log"; }
while read -r user pass; do
  case "$user" in
    mm-demo)  run market_maker.py "$user" "$pass" ;;
    arb-demo) run etf_arb.py      "$user" "$pass" ;;
    opt-demo) run options_bot.py  "$user" "$pass" ;;
  esac
done < .accounts
wait
