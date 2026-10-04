# QFin Exchange Bot SDK

Write a Python bot that trades fruit, the fruit ETF and up-down options on the QFin exchange.

## Setup

```bash
pip install requests websockets
python my_bot.py -u YOUR_TEAM -p YOUR_PASSWORD
```

Your account is created the first time you log in. Options: `--url http://host:3211` (or set
`EXCHANGE_URL`), `--tick 500` (ms between `on_tick` calls, default 1000).

## A bot

```python
from exchange import Exchange, Buy, Sell, CancelAll

class MyBot(Exchange):
    def on_tick(self):
        actions = [CancelAll()]
        for f in self.fruits:
            mid = self.mid(f)
            if mid is None:
                continue
            actions += [Buy(f, self.snap(f, mid - 0.03), qty=5),
                        Sell(f, self.snap(f, mid + 0.03), qty=5)]
        self.batch(actions)          # one round trip for everything

MyBot().run()
```

## Products

| Symbols | What | Notes |
|---|---|---|
| `apples` `bananas` `oranges` `strawberries` `watermelon` | Fruit | 1c tick. Trade like futures: margined, you can be short |
| `etf` | 1 of each fruit | Create / redeem against the fruit for a fee (`self.config["etfFee"]` per unit) |
| `up5` `up60` | Up-down options on the ETF | Pays $1 if the ETF **last trade** at window close >= strike (tie = up), else $0. Price 0.01-0.99 |

Option symbols roll: `up5` always means the *current* 5 minute window. Windows line up with the
clock (xx:00, xx:05, ...). The strike is the ETF last trade at the window open; each window's
settle is the next window's strike. At the close, resting orders are cancelled and positions pay
out in cash. There is only an "up" book: selling up at `p` is buying down at `1 - p`.

## Money rules

- Every trading day starts with `starting_cash` ($10,000). At the end of the day your P&L is
  recorded and you are reset to flat with fresh cash.
- **Margin**: fruit and ETF need `margin rate x price x |position + resting orders|`; long options
  need the premium, short options `1 - price` per contract. Orders that need more margin than your
  equity are rejected (`insufficient margin`).
- **Busts**: if your equity hits 0 you are reset mid-day. The loss still counts for the day.
- **Score** = mean daily P&L - standard deviation of daily P&L. Consistency beats luck.
- Orders bigger than your position limit are cut down to the limit.

## Reference

Market data (live, updated over WebSocket):

| | |
|---|---|
| `products`, `fruits`, `etfs`, `options` | Symbol lists |
| `kind(s)` | `"fruit"`, `"etf"` or `"option"` |
| `mid(s, default=None)` | Mid, else last trade, else default |
| `best_bid(s)` `best_ask(s)` `best_bid_qty(s)` `best_ask_qty(s)` `spread(s)` | Top of book |
| `book(s)` | Full `OrderBook` (bids, asks, last_trade_price) |
| `last_trade(s)` | Last trade price |
| `fair_value("etf")` | Sum of the fruit mids (naive) |
| `etf_composition("etf")` | List of `EtfComponent(symbol, weight)` |
| `tick_size(s)`, `snap(s, price)`, `position_limit(s)` | Price grid helpers (`snap` also clamps options to 0.01-0.99) |
| `option_info("up5")` | `OptionInfo`: strike, window_open, window_close, seconds_left, price_source |
| `option_windows(expiry_min=5, limit=100)` | Settled windows (strike, settle, up_won). HTTP call |
| `server_time` | Exchange clock, epoch seconds |
| `config` | phase, startingCash, dayLengthMin, etfFee, marginRates, enabledExpiries |

Your account:

| | |
|---|---|
| `account` | `Account`: cash, equity, pnl, margin_used, margin_available, busts, day, day_end |
| `pnl` | Today's P&L |
| `position(s)`, `positions` | Signed position / dict of non-zero positions |
| `open_orders(s=None)` | Your resting orders |

Trading:

| | |
|---|---|
| `buy(s, price, qty, ioc=False)` / `sell(...)` | Returns `OrderResult` (order_id, status, fills, filled_qty, avg_price) |
| `cancel(order_id)`, `cancel_all(s=None)` | |
| `batch([CancelAll(), Buy(...), Sell(...), Cancel(id)])` | Many actions in one request |
| `create_etf("etf", qty)` / `redeem_etf("etf", qty)` | Fruit <-> ETF, returns `{"qty", "fee", ...}` |

Data:

| | |
|---|---|
| `download_data("data")` | Fetch the exchange's recorded Parquet files (same layout and columns as the handout data) |

```python
from exchange import Exchange
Exchange().connect("myteam", "mypassword").download_data("data")
```

Logging: `self.log(msg)`, `self.warn(msg)`.

## Not using Python?

The SDK is a thin wrapper over a plain HTTP + WebSocket API. The full reference is in `API.md`
(ask the organisers for it if it isn't in your SDK zip).

## Tips

- There is a lot of uninformed flow. Market making works, but watch your inventory.
- The house option market maker is not perfect. Think about what settlement actually uses.
- Speed matters: use `batch()`, and don't make HTTP calls you don't need every tick.
- See `example_bot.py` for every feature in one (deliberately bad) bot.
