# QFin Mini Exchange (2026 Semester 2 project)

The mock exchange for the QFin trading project described in
[`docs/QFin Project Overview -- Data.pdf`](docs/). Teams write bots that trade five fruits, a fruit
ETF and up-down options on the ETF, against each other and against simulated house bots.

**Live:** https://exchange.qfinuwa.org (QFin VPS). Admin login is shared separately.

> **Private repo.** `server/SIMULATION.md` and `server/sim.go` describe the hidden fair value
> processes ("the patterns") and the house bots. Don't share them with students. Give students
> only the `bot-sdk/` folder.

## What's where

| Path | What |
|---|---|
| `server/` | Go exchange: matching engine, accounts, margin, trading days, options, scoring, market data recorder, house bots. Single process, state saved to `exchange_data.json`. Same binary has `backtest` and `runner` modes for uploaded bots |
| `src/` | Next.js web UI: trading screen, leaderboard, positions, prices, data downloads, admin |
| `bot-sdk/` | Python SDK handed to students (`exchange.py`, `README.md`, examples) |
| `demo-bots/` | Three working bots (market maker, ETF arb, options) for testing the exchange. Organiser only |
| `deploy/` | Dockerfiles, compose file and `deploy.sh` for the QFin VPS |
| `docs/API.md` | **Full HTTP + WebSocket API reference** (every endpoint, request/response examples, errors) |
| `PROJECT.md` | The rules (products, margin, days, scoring) and the original API contract |
| `server/SIMULATION.md` | How the market is simulated. **Organiser only** |

## How the market works (short version)

- **Fruits** (`apples`, `bananas`, `oranges`, `strawberries`, `watermelon`) each have a hidden fair
  value with a built-in pattern (mean reversion, momentum, lead-lag, jumps, a cycle).
- **ETF** = 1 of each fruit. Can be created/redeemed for a fee.
- **Up-down options** `up5`, `up60` on the ETF. Pay $1 if the ETF last trade at window close >=
  strike. `up15` exists but is off by default.
- House bots provide the market: an uninformed market maker, random flow, informed traders that
  pull prices toward fair value, and an option market maker with a deliberately naive model.
- Every team starts each day with $10k, fruits/ETF are margined like futures, going to 0 equity is a
  "bust" (reset, loss still counts). **Score = mean daily P&L - std of daily P&L.**
- Every second of market data is recorded to Parquet with the same tables and columns as the
  handout data (Data page, or `self.download_data()` in the SDK).

## Uploaded bots (how teams are scored)

Live trading can't score a bot properly (it would take weeks of real time), so teams upload their
bot on the **Bots** page and the server backtests it:

- `server/backtest.go`: runs the real exchange code (house bots, matching, margin, days, options)
  on a simulated clock, `backtestDays` days of `backtestDayMin` minutes (admin settings, default
  20 x 60), as fast as the bot allows (~2000 simulated seconds per second for a simple bot on a
  desktop). The bot is the normal SDK bot in a Python subprocess; the SDK sees `QFIN_BACKTEST=1`
  and sends its API calls over pipes into the real HTTP handlers. `on_tick` runs once per
  simulated second. Fair values use their own RNG stream seeded by `backtestSeed`, so every team
  sees the same fair value paths.
- `server/submissions.go`: upload / list / leaderboard endpoints. Uploads go to `bots/<id>/`.
- `server/runner.go` (`exchange-runner` container): picks up queued uploads one at a time and runs
  each bot as `nobody` in a private folder, with no network, prlimits, and timeouts (5 s per tick,
  30 min per run). Bots can't see other uploads or the exchange state.
- For the final ranking, set a fresh `backtestSeed` (and more days) on the admin page and press
  **Re-run all bots**.

Try one locally without the queue:

```bash
cd server && go run . backtest -bot ../bot-sdk/my_bot.py -sdk ../bot-sdk -days 2 -day-min 60
```

## Running a competition (admin page)

1. Log in as the admin at https://exchange.qfinuwa.org and open **Admin**.
2. **Open Exchange** starts trading and the house bots (they only run while open).
3. **Competition Rules**: phase (training/testing), starting cash, day length, ETF fee, margin
   rates, which option expiries exist, click trading on/off, house bots on/off.
4. **End day now** settles options, records everyone's day P&L and resets accounts. Days also end
   automatically every `dayLengthMin` (default: UTC midnight).
5. Leaderboard ranks by score once at least one day has ended.

The first account ever registered on a fresh server becomes admin; everyone else is a normal team.
Teams create their account just by logging in (in the UI or from a bot).

## Giving teams the SDK

Zip the `bot-sdk/` folder and share it. Teams need `pip install requests websockets` and run
`python my_bot.py -u TEAM -p PASSWORD`. It connects to https://exchange.qfinuwa.org by default
(`--url` to override). Everything is documented in `bot-sdk/README.md`.

## Local development

```bash
# exchange server (port 3211); state + data are written to the current directory
cd server && go run .

# web UI (port 3000) pointed at the local server
npm install
NEXT_PUBLIC_EXCHANGE_URL=http://localhost:3211 npm run dev

# three demo bots against the local server (accounts are created on first run)
demo-bots/run_all.sh

# backtest runner for the Bots page (no sandbox locally: -uid 0)
cd server && go run . runner -jobs bots -work /tmp/bt-work -sdk ../bot-sdk -uid 0 -owner 0
```

Register in the UI first to become the local admin, then open the exchange from the Admin page.

## Deploying

`deploy/deploy.sh` builds both Docker images and ships them to the VPS. See
[`deploy/README.md`](deploy/README.md).
