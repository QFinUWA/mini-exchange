"""
Demo bot 3: up-down options. Prices "up" with a normal model on the ETF *last trade*
(what settlement actually uses), with volatility estimated from recent ETF moves,
and takes any quote that is mispriced by more than EDGE.

Run:  python options_bot.py -u opt-demo -p <password> --url http://localhost:3211
"""
import math, os, sys
from collections import deque
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "bot-sdk"))
from exchange import Exchange

EDGE = 0.05
QTY = 10
MAX_POS = 150


def norm_cdf(x):
    return 0.5 * (1 + math.erf(x / math.sqrt(2)))


class OptionsBot(Exchange):
    def __init__(self):
        super().__init__()
        self.etf_hist = deque(maxlen=600)   # (time, price) samples

    def vol_per_sqrt_sec(self):
        if len(self.etf_hist) < 30:
            return 0.012
        pts = list(self.etf_hist)
        moves = [(b[1] - a[1]) ** 2 / max(b[0] - a[0], 1e-3) for a, b in zip(pts, pts[1:])]
        return max(math.sqrt(sum(moves) / len(moves)), 0.002)

    def on_tick(self):
        etf = self.last_trade("etf")
        if etf is None:
            return
        self.etf_hist.append((self.server_time, etf))
        vol = self.vol_per_sqrt_sec()

        for symbol in self.options:
            info = self.option_info(symbol)
            if info is None or info.strike is None or info.seconds_left < 2:
                continue
            sd = vol * math.sqrt(info.seconds_left)
            p_up = norm_cdf((etf - info.strike) / sd)    # tie counts as up; ignore
            pos = self.position(symbol)
            bid, ask = self.best_bid(symbol), self.best_ask(symbol)

            if ask is not None and p_up - ask > EDGE and pos < MAX_POS:
                self.buy(symbol, ask, min(QTY, self.best_ask_qty(symbol)), ioc=True)
                self.log(f"{symbol} buy up @ {ask:.2f}, model {p_up:.2f} ({info.seconds_left:.0f}s left)")
            elif bid is not None and bid - p_up > EDGE and pos > -MAX_POS:
                self.sell(symbol, bid, min(QTY, self.best_bid_qty(symbol)), ioc=True)
                self.log(f"{symbol} sell up @ {bid:.2f}, model {p_up:.2f} ({info.seconds_left:.0f}s left)")

        self.log(f"etf {etf:.2f} vol {vol:.4f}  pnl {self.account.pnl:+.2f}  pos {self.positions}")


OptionsBot().run(tick_ms=500)
