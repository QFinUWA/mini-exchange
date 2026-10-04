# QFin 2026 Sem 2 Project - Exchange Implementation

This exchange implements the "QFin Project Overview -- Data" handout. This file is the
contract between the Go server (`server/`), the Next.js UI (`src/`) and the Python bot SDK
(`bot-sdk/`). Organiser-only simulation details (fair value processes, house bots) live in
`server/SIMULATION.md` and must NOT be shipped to students.

## Products

All products are created automatically on server start (idempotent). Symbols match the data.

| Symbol | Kind | Tick | Notes |
|---|---|---|---|
| `apples`, `bananas`, `oranges`, `strawberries`, `watermelon` | `fruit` | 0.01 | Trade as futures (margined, no cash outlay rule, see Accounts) |
| `etf` | `etf` | 0.01 | Basket of 1 of each fruit. Create/redeem via `/api/etf-swap` for a fee |
| `up5`, `up60` | `option` | 0.01 | Up-down binary on the ETF. Price in [0.01, 0.99]. `up15` exists but is disabled by default |

Option products are *rolling*: the symbol stays `up5` / `up60`, but the contract behind it is the
current window. Windows are aligned to the clock (`window_open % (expiry*60) == 0`, UTC).

- **strike** = ETF price at the window open second, **settle** = ETF price at the close second.
- `up5`, `up60`: price source `etf_last_trade` (last ETF trade at or before that second).
- `up15`: price source `etf_mid` (ETF mid at the end of that second).
- Up pays $1 if `settle >= strike` (tie = up), else $0. Each window's settle is the next window's strike.
- At the close second: all resting orders on that option are cancelled, every position is cash
  settled at 1 or 0, and the next window opens.
- Selling up at `p` is the same as buying down at `1 - p`. There is only an up book.

## Accounts, days and scoring

- Every user starts each trading day with `startingCash` ($10,000).
- `cash` moves by `-price*qty` on buys and `+price*qty` on sells (and fees, and option payouts).
- `equity = cash + sum(qty * mark)`, mark = book mid, else last trade. `pnl = equity - startingCash`.
- **Margin** (checked on every order and ETF swap, against the worst case of position + resting
  orders + the new order):
  - fruit / etf: `marginRate[kind] * mark * |worst-case qty|` (defaults: fruit 0.20, etf 0.20)
  - option long: `qty * mark` (premium at risk); option short: `|qty| * (1 - mark)`
  - Order rejected with `insufficient margin` if `requiredMargin > equity`.
- **Bust**: if `equity <= 0` at any time, the user is reset to `startingCash`, flat, orders
  cancelled, `busts` += 1 (training phase behaviour from the handout).
- **Day end** (every `dayLengthMin`, default 1440 = UTC midnight, or admin "End day now"):
  options settle first, then each user's `pnl` is recorded as a daily result, then everyone is
  reset to `startingCash` and flat, and all user orders are cancelled.
- **Score** = mean(daily pnl) - std(daily pnl) (population std; 0 with one day) - SIG algothon style.
  Today's live pnl is NOT included in the score until the day ends.

## Data recording (same schema as the handout)

The server records every second and writes Parquet, one file per UTC day:
`server/data/<table>/date=YYYY-MM-DD.parquet` for tables `top_of_book`, `trades`,
`option_top_of_book`, `option_trades`, `option_windows`. Columns exactly as in the handout
(ts in epoch seconds, prices in dollars; trades.price is the average fill price of the
aggressing order; aggressor `buy`/`sell` or `buy_up`/`sell_up`). Today's file is rewritten
every minute; past days are final.

## HTTP API (additions and changes)

Existing endpoints are unchanged unless noted. Auth is `Authorization: Bearer <token>`.

### Product objects (in `/api/state`, WS `state`, `/api/products`)
```jsonc
{
  "id": "p_3", "symbol": "up5", "name": "ETF Up 5m", "isEtf": false,
  "kind": "option",                // "fruit" | "etf" | "option"
  "positionLimit": 5000, "tickSize": 0.01,
  "compositions": [[{"symbol":"apples","weight":1}, ...]],   // etf only
  "option": {                      // option only
    "expiryMin": 5,
    "windowOpen": 1790000000,      // epoch seconds
    "windowClose": 1790000300,     // epoch seconds
    "strike": 93.12,               // null until the first ETF trade exists
    "priceSource": "etf_last_trade"
  }
}
```

### State (`GET /api/state` and WS `state` message) - new fields
```jsonc
{
  "serverTime": 1790000123456,     // ms
  "account": {
    "cash": 9876.5, "equity": 10012.3, "pnl": 12.3, "startingCash": 10000,
    "marginUsed": 812.0, "marginAvailable": 9200.3,
    "busts": 0,                    // today
    "day": "2026-09-30", "dayStart": 1790000000000, "dayEnd": 1790086400000  // ms
  },
  "config": {
    "phase": "training", "startingCash": 10000, "dayLengthMin": 1440,
    "etfFee": 0.05,                // $ per ETF unit created or redeemed
    "marginRates": {"fruit": 0.2, "etf": 0.2},
    "enabledExpiries": [5, 60],
    "clickTrading": true           // web UI click-to-trade on book levels (UI-only switch)
  },
  "optionWindows": [               // most recent settled windows first, max 30
    {"expiryMin":5,"windowOpen":1789999700,"strike":93.1,"settle":93.4,"upWon":true,"priceSource":"etf_last_trade"}
  ],
  "pnl": 12.3                      // same as account.pnl (kept for compatibility)
  // ...existing: exchangeOpen, products, books, positions, openOrders
}
```
WS `state` also carries `recentTrades`, `leaderboard`, `allPositions`, `allUsers` as before.

### Leaderboard entries - new fields
```jsonc
{"userId":"u_5","username":"alice","totalPnl":12.3,"realizedPnl":4,"unrealizedPnl":8.3,
 "days":3,"meanDailyPnl":150.2,"stdDailyPnl":80.1,"score":70.1,"busts":1}
```
Sorted by `score` desc when any user has a completed day, else by `totalPnl`. House bots are
excluded from leaderboard, allPositions and allUsers.

### New endpoints
- `GET /api/option-windows?expiry=5&limit=100` - settled windows, newest first.
- `GET /api/daily-results` - `{ "<username>": [{"day":"2026-09-30","pnl":12.3,"busts":0}] }`
- `GET /api/config` - same object as `state.config`.
- `POST /api/admin/config` - partial update, any of: `etfFee`, `fruitMarginRate`, `etfMarginRate`,
  `startingCash`, `dayLengthMin`, `phase`, `simEnabled` (bool), `enabledExpiries` ([5,15,60] subset),
  `clickTrading` (bool; the web UI hides click-to-trade when false, the API still accepts orders).
  Returns the full config (admin also sees `simEnabled`).
- `POST /api/admin/end-day` - end the trading day now.
- `GET /api/data` - list of recorded files `[{"table":"trades","date":"2026-09-30","path":"trades/date=2026-09-30.parquet","bytes":1234}]`
- `GET /api/data/<table>/date=YYYY-MM-DD.parquet` - download a file.

### Changed endpoints
- `POST /api/etf-swap` - charges `etfFee * qty` from cash. Response adds `"fee": <total fee>`.
- Orders: option prices must be within [0.01, 0.99]. Margin check as above.
