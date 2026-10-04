# Market simulator (ORGANISER ONLY - do not ship to students)

Code: `sim.go`. The simulator runs inside the Go server while the exchange is open and
`simEnabled` is on (admin config). All house bots trade through the normal matching engine as
users `house-mm`, `house-flow`, `house-informed`, `house-options`. House users are hidden from
the leaderboard and user lists, can't log in, and skip margin, position limits and rate limits.
Their trades show up in the public trade feed and in the recorded data like anyone else's.

Parameters are calibrated to the handout data (sampled on 2026-06-25):

| | Data | Sim |
|---|---|---|
| Fruit price level | $8 - $14 | anchors 11 / 9 / 12 / 10 / 13.5 |
| Fruit spread / top size | 6c / ~8 | 6-7c / 8 |
| ETF spread / top size | 14c / 1 | 14c / 2 |
| Fruit trades/s, size median / mean / max | 0.75, 10 / 14.5 / 300 | ~0.75, lognormal median 10 |
| ETF trades/s, size | 0.33, median 3 / mean 6 | ~0.3, lognormal median 3 |
| Fruit mid sd 60s / 1h | 0.06 / 0.25 - 0.7 | similar |
| Option spread / size | 7c / 50, always quoted | 7c / 50 (+100 one level out) |
| Option trades/s (5m / 15m / 60m) | 0.15 / 0.08 / 0.05, size median 10-12, max 100 | same |

## Fair value processes (one step per second, `simSecond`)

These are the "patterns by design". They are NOT the ones in the handout data (that generator
isn't in this repo). If you have the original generator, replace `simSecond` with it.

- **apples**: Ornstein-Uhlenbeck, reverts to a slowly wandering level with a ~15 min time
  constant (`/900`), noise 0.006/s.
- **bananas**: momentum. Persistent drift velocity (AR(1), phi = 0.98) plus noise.
- **oranges**: lead-lag. Each second oranges moves by apples' move from 20 seconds earlier,
  plus its own noise.
- **strawberries**: jumps (about one per 15 min, sd $0.25) that half revert over the following
  minutes.
- **watermelon**: random walk plus a deterministic 15 minute sine cycle (amplitude $0.25,
  phase locked to the UTC clock).
- **etf**: fair value = sum of the five.

All have a very weak pull to their anchor so prices stay in range over months.

## House bots (`SimTick`, every 250 ms)

- **house-mm** (uninformed): quotes 3 levels each side around its own reference price. The
  reference only moves with the flow it trades against (`lambda` per unit of inventory change),
  plus an inventory skew. It never sees fair value. This is why prices are "dictated by order flow".
- **house-flow** (uninformed): Poisson marketable orders with lognormal sizes, plus some resting
  orders near the touch that are cancelled after 30 s.
- **house-informed**: sees fair value with noise and trades the mid back towards it when it is
  off by more than ~1.7 half spreads. Mispricings smaller than that persist, and noise flow
  pushes the price away from fair value, which then decays.
- **house-options**: prices up contracts with a naive normal model on the ETF *mid*, with a
  fixed vol of 0.012 $/sqrt(s), shifted by the flow it trades against. It ignores the fruit
  patterns and that settlement uses the ETF *last trade*. That's where the option edge is.
  Option noise flow comes from house-flow, and a weak option-informed trader uses the true ETF
  fair value.

Tuning knobs are `simParams`, `optSimParams` and the constants in `simSecond`. Sim state
(fair values, MM references) is saved in `exchange_data.json`, so restarts continue seamlessly.
