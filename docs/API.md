# Exchange API reference

Base URL: `https://exchange.qfinuwa.org` (local: `http://localhost:3211`). Everything is JSON.
The Python SDK (`bot-sdk/`) wraps all of this; you only need this page to write a client in
another language or to understand what the SDK does.

- [Conventions](#conventions)
- [Market rules that affect the API](#market-rules-that-affect-the-api)
- [Auth](#auth)
- [Live state (WebSocket)](#live-state-websocket)
- [Trading](#trading): order, cancel, cancel-all, batch, ETF swap
- [Market and account data](#market-and-account-data)
- [Admin](#admin)
- [Errors](#errors)

## Conventions

- Authenticated endpoints take `Authorization: Bearer <token>` (token from `/api/auth`).
- Products are addressed by **symbol**: `apples`, `bananas`, `oranges`, `strawberries`,
  `watermelon`, `etf`, `up5`, `up60` (and `up15` if enabled).
- Prices are dollars, tick size 0.01. Option prices must be within [0.01, 0.99].
  Quantities are whole units.
- Times: `serverTime`, `createdAt`, `dayStart`, `dayEnd`, `endedAt` are **epoch milliseconds**;
  option `windowOpen` / `windowClose` are **epoch seconds**.
- CORS is open (`*`), so browser clients work from any origin.
- Errors are `{"error": "message"}` with a 4xx status. Batch actions report errors per action.

## Market rules that affect the API

Full rules: [`PROJECT.md`](../PROJECT.md). The ones a client trips over:

- **Exchange must be open.** Orders, batches and ETF swaps fail with `exchange is closed`
  otherwise. Check `exchangeOpen` in the state.
- **Rate limit**: one order/cancel request per 10 ms per user (a batch counts as one). Exceeding it
  returns `rate limited (10ms cooldown)`. Use `/api/batch` to send many actions at once.
- **Position limits**: fruits 500, ETF 200, options 5000. An order that would go past the limit
  (counting your resting orders on the same side) is **cut down** to fit; it's only rejected if no
  quantity fits.
- **Margin**: every order is checked against the worst case of position + resting orders + the new
  order. Rejected with `insufficient margin (required X, equity Y)` if it needs more than your equity.
- **Matching**: price-time priority, fills at the resting order's price. If your order would cross
  your own resting order, both are reduced by the overlapping quantity instead of trading (no
  self-trades).
- **Days**: at day end options settle, P&L is recorded, and every account is reset to starting cash
  with no positions or orders. Busting (equity <= 0) does the same mid-day.
- **Options** roll: `up5` always refers to the current window. At window close all `up5` orders are
  cancelled and positions pay out 1 or 0 in cash.

## Auth

### `POST /api/auth`

Logs in, or creates the account if the username doesn't exist yet. The very first account on a
fresh exchange becomes admin.

```json
{"username": "team-alpha", "password": "..."}
```
```json
{"token": "9f2c...", "userId": "u_15", "username": "team-alpha", "isAdmin": false}
```

The token doesn't expire and stays the same across logins. Wrong password for an existing
username returns `401 {"error": "username already taken"}`.

## Live state (WebSocket)

`wss://exchange.qfinuwa.org/ws` (local `ws://localhost:3211/ws`)

1. Connect and send `{"type": "auth", "token": "<token>"}`.
2. The server replies with `{"type": "state", "data": {...}}` immediately, then again **every second**
   and after every change (trades, orders, settlements). Each message is a full snapshot, not a diff.
3. A bad token gets `{"type": "error", "error": "invalid session"}`.

The `data` object is the same as [`GET /api/state`](#get-apistate) plus `recentTrades`,
`leaderboard`, `allPositions` (and `allUsers` for admins).

## Trading

### `POST /api/order` - place an order

```json
{"symbol": "apples", "side": "buy", "price": 10.95, "qty": 5, "orderType": "limit"}
```

| Field | |
|---|---|
| `side` | `buy` or `sell` |
| `orderType` | `limit` (default, rests if not filled), `ioc` (fill what you can now, cancel the rest), `postOnly` (cancelled if it would trade immediately) |

```json
{"orderId": "o_8762", "status": "filled", "fills": [{"price": 10.95, "qty": 5}]}
```

`status`: `filled`, `partial` (some filled, rest is resting), `open` (nothing filled, resting),
`cancelled` (IOC remainder, post-only that would cross, or self-trade cancelled). `fills` is `null`
when nothing traded.

For options, "selling up at p" is how you buy down: there is only an up book.

### `DELETE /api/order` - cancel one order

```json
{"orderId": "o_8762"}
```
```json
{"success": true}
```

### `POST /api/cancel-all`

All fields optional: `{"symbol": "apples", "side": "buy"}`. Returns `{"cancelled": 3}`.

### `POST /api/batch` - many actions, one request

Runs in order, atomically with respect to other users (nobody can trade in between), and counts as
one request for the rate limit. Use `symbol` (or `productId`) to address products.

```json
{"actions": [
  {"type": "cancelAll"},
  {"type": "cancelAll", "symbol": "apples", "side": "sell"},
  {"type": "cancel", "orderId": "o_8762"},
  {"type": "order", "symbol": "apples", "side": "buy", "price": 10.94, "qty": 5},
  {"type": "order", "symbol": "apples", "side": "sell", "price": 11.02, "qty": 5, "orderType": "ioc"},
  {"type": "etfSwap", "symbol": "etf", "qty": 2, "direction": "create"}
]}
```

Response: one result per action, in the same order.

```json
[
  {"ok": true, "data": {"cancelled": 4}},
  {"ok": true, "data": {"cancelled": 0}},
  {"ok": false, "error": "order not found"},
  {"ok": true, "data": {"orderId": "o_9001", "status": "open", "fills": null}},
  {"ok": false, "error": "insufficient margin (required 10450.00, equity 10000.00)"},
  {"ok": true, "data": {"success": true, "qty": 2, "fee": 0.1, "fairValue": 55.71, "direction": "create"}}
]
```

If the whole batch is rejected (closed exchange, rate limit, bad token) the response is a single
`[{"ok": false, "error": "..."}]`.

### `POST /api/etf-swap` - create / redeem ETF

```json
{"symbol": "etf", "qty": 2, "direction": "create"}
```
```json
{"success": true, "qty": 2, "fee": 0.1, "fairValue": 55.71, "direction": "create"}
```

- `create`: gives up 1 of each fruit per ETF. You must **hold** the fruit (long positions); `qty` is
  reduced to what you hold and to the ETF position limit.
- `redeem`: gives up ETF for 1 of each fruit; reduced to the ETF you hold and the fruit limits.
- The fee (`config.etfFee` per unit, default $0.05) is taken from cash. The response `qty` is what
  actually happened (0 if nothing could be swapped; that's still `success: true`).
- Internal transfer prices are the current fruit mids (`fairValue` is their sum). This has no effect
  on equity, only on how P&L is split between realized and unrealized.

## Market and account data

### `GET /api/state`

Auth required. The snapshot your bot works from (also what the WebSocket sends).

```jsonc
{
  "exchangeOpen": true,
  "serverTime": 1791125132931,
  "products": [
    {"id": "p_5", "symbol": "apples", "name": "Apples", "kind": "fruit", "isEtf": false,
     "positionLimit": 500, "tickSize": 0.01},
    {"id": "p_10", "symbol": "etf", "name": "Fruit ETF", "kind": "etf", "isEtf": true,
     "positionLimit": 200, "tickSize": 0.01,
     "compositions": [[{"symbol": "apples", "weight": 1}, {"symbol": "bananas", "weight": 1}, ...]]},
    {"id": "p_11", "symbol": "up5", "name": "ETF Up/Down 5m", "kind": "option", "isEtf": false,
     "positionLimit": 5000, "tickSize": 0.01,
     "option": {"expiryMin": 5, "windowOpen": 1791124500, "windowClose": 1791124800,
                "strike": 55.73, "priceSource": "etf_last_trade"}}
  ],
  "books": {
    "apples": {"bids": [{"price": 10.98, "qty": 8, "orderCount": 1}, ...],   // best first
               "asks": [{"price": 11.05, "qty": 8, "orderCount": 1}, ...],   // best first
               "lastTradePrice": 11.04}
  },
  "positions": [
    {"productId": "p_5", "symbol": "apples", "qty": 6, "avgEntryPrice": 11.04,
     "realizedPnl": -0.05, "unrealizedPnl": 0.12}
  ],
  "openOrders": [
    {"_id": "o_8764", "productId": "p_10", "symbol": "etf", "side": "buy", "price": 50,
     "qty": 200, "filledQty": 0, "status": "open", "orderType": "limit", "createdAt": 1791124397548}
  ],
  "account": {
    "cash": 9765.46, "equity": 9999.19, "pnl": -0.81, "startingCash": 10000,
    "marginUsed": 2280.71, "marginAvailable": 7718.48, "busts": 0,
    "day": "2026-10-04", "dayStart": 1791072000000, "dayEnd": 1791158400000
  },
  "config": {"phase": "training", "startingCash": 10000, "dayLengthMin": 1440, "etfFee": 0.05,
             "marginRates": {"fruit": 0.2, "etf": 0.2}, "enabledExpiries": [5, 60],
             "clickTrading": true},
  "optionWindows": [   // last 30 settled windows, newest first
    {"expiryMin": 5, "windowOpen": 1791124200, "strike": 55.5986, "settle": 55.73,
     "upWon": true, "priceSource": "etf_last_trade"}
  ],
  "pnl": -0.81         // same as account.pnl
}
```

Notes: `equity = cash + sum(qty x mark)` where mark is the book mid, else the last trade.
`pnl` includes losses from earlier busts today. An option `strike` is `null` until the ETF has
traded. The ETF trade price used for strikes can be off the 1c grid (it is the average fill price of
the aggressing order).

### Public endpoints (no auth)

| Endpoint | Returns |
|---|---|
| `GET /api/config` | Same object as `state.config` (admins also get `simEnabled`) |
| `GET /api/products` | Active products: `_id`, `symbol`, `name`, `kind`, `isEtf`, `positionLimit`, `tickSize`, `isActive`, plus `etfCompositions` or `option` |
| `GET /api/exchange-state` | `{"isOpen": true}` |
| `GET /api/option-windows?expiry=5&limit=100` | Settled windows, newest first. `expiry` optional (5/15/60), `limit` up to 5000 |
| `GET /api/leaderboard` | `[{userId, username, totalPnl, realizedPnl, unrealizedPnl, days, meanDailyPnl, stdDailyPnl, score, busts}]`, sorted by `score` once any day has ended, else by today's `totalPnl` |
| `GET /api/daily-results` | `{"team-alpha": [{"day": "2026-10-04", "pnl": 12.3, "busts": 0, "endedAt": 1791158400000}]}` |
| `GET /api/trades` | Last 100 trades, newest first: `{_id, symbol, price, qty, createdAt, buyer, seller, isMine, mySide}`. `isMine`/`mySide` are filled in if you send your token |
| `GET /api/pnl-history` | `{"team-alpha": [{"snapshotAt": ms, "totalPnl": 12.3, "realizedPnl": 4}]}`, one point per 5 s |
| `GET /api/price-history` | `{"apples": [{"snapshotAt": ms, "mid": 10.95}]}`, one point per 5 s |
| `GET /api/all-positions` | Everyone's non-zero positions: `{username, symbol, qty, avgEntryPrice, realizedPnl, unrealizedPnl}` |
| `GET /api/volume-matrix` | Traded volume between each pair of users: `{buyer, seller, volume, qty}` |
| `GET /api/data` | Recorded Parquet files: `[{"table": "trades", "date": "2026-10-04", "path": "trades/date=2026-10-04.parquet", "bytes": 13579}]` |
| `GET /api/data/<path>` | Download one file, e.g. `/api/data/trades/date=2026-10-04.parquet` |

House bots (`house-*` users) are left out of the leaderboard, positions and user lists, but their
trades appear in `/api/trades` and the recorded data.

### Recorded data (Parquet)

One file per UTC day per table, same columns as the handout data. Today's files are rewritten every
minute; earlier days are final.

| Table | Columns |
|---|---|
| `top_of_book` | `ts, product, bid, ask, bid_size, ask_size` (every second, end-of-second snapshot) |
| `trades` | `ts, product, price, size, aggressor` (`buy`/`sell`; price = average fill of the aggressing order) |
| `option_top_of_book` | `ts, expiry_min, window_open, bid, ask, bid_size, ask_size` (up contract) |
| `option_trades` | `ts, expiry_min, window_open, price_up, size, aggressor` (`buy_up`/`sell_up`) |
| `option_windows` | `expiry_min, window_open, strike, settle, up_won, price_source` |

## Admin

All require an admin token. All are `POST` except `GET /api/admin/users`.

| Endpoint | Body | Effect |
|---|---|---|
| `/api/admin/toggle` | none | Open/close the exchange. Returns `{"isOpen": bool}`. House bots only trade while open |
| `/api/admin/config` | any subset of `etfFee`, `fruitMarginRate`, `etfMarginRate`, `startingCash`, `dayLengthMin`, `phase` (`training`/`testing`), `simEnabled`, `enabledExpiries` (subset of `[5,15,60]`), `clickTrading` | Returns the full config. Disabling an expiry cancels its orders and refunds positions at entry price |
| `/api/admin/end-day` | none | End the trading day now (settle, record P&L, reset everyone) |
| `/api/admin/reset` | `{"targetUserId": "u_15"}` or `{}` | Reset one user, or **everything** if no target |
| `/api/admin/users` (GET) | | All users with `isAdmin`, `noPositionLimit`, `rateLimitMs` |
| `/api/admin/no-pos-limit` | `{"targetUserId": "u_15", "value": true}` | Exempt a user from position limits |
| `/api/admin/rate-limit` | `{"targetUserId": "u_15", "rateLimitMs": 50}` | Per-user rate limit (default 10 ms) |
| `/api/admin/product` | `{"action": "updatePositionLimit", "productId": "p_5", "positionLimit": 300}` | Also `updateTickSize`, `create`, `createEtf`, `delete`. The project products are recreated on restart, so don't delete them |

## Errors

| Status | When |
|---|---|
| 400 | Rule violations: `exchange is closed`, `insufficient margin (...)`, `position limit (500) reached`, `price must be a multiple of tick size 0.01`, `option price must be between 0.01 and 0.99`, `rate limited (10ms cooldown)`, `order not found`, `admin required`, ... |
| 401 | Missing/invalid token, or wrong password on login (`username already taken`) |
| 404 | `unknown symbol: xyz`, unknown data file |
| 405 | Wrong HTTP method |

## Minimal client (no SDK)

```python
import requests, json
from websockets.sync.client import connect

BASE = "https://exchange.qfinuwa.org"
tok = requests.post(f"{BASE}/api/auth", json={"username": "team-alpha", "password": "..."}).json()["token"]
H = {"Authorization": f"Bearer {tok}"}

with connect(BASE.replace("https", "wss") + "/ws") as ws:
    ws.send(json.dumps({"type": "auth", "token": tok}))
    for raw in ws:
        state = json.loads(raw)["data"]
        book = state["books"]["apples"]
        if state["exchangeOpen"] and book["bids"] and book["asks"]:
            mid = (book["bids"][0]["price"] + book["asks"][0]["price"]) / 2
            requests.post(f"{BASE}/api/batch", headers=H, json={"actions": [
                {"type": "cancelAll", "symbol": "apples"},
                {"type": "order", "symbol": "apples", "side": "buy",  "price": round(mid - 0.03, 2), "qty": 5},
                {"type": "order", "symbol": "apples", "side": "sell", "price": round(mid + 0.03, 2), "qty": 5},
            ]})
```
