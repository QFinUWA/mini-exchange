"""
Demo bot 1: market maker. Quotes one tick inside the touch on every fruit and the ETF,
skewing quotes against inventory so it doesn't build up a big position.

Run:  python market_maker.py -u mm-demo -p <password> --url http://localhost:3211
"""
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "bot-sdk"))
from exchange import Exchange, Buy, Sell, CancelAll

MAX_POS = 40
SIZE = 4


class MarketMaker(Exchange):
    def on_tick(self):
        actions = [CancelAll()]
        for symbol in self.fruits + self.etfs:
            bid, ask = self.best_bid(symbol), self.best_ask(symbol)
            if bid is None or ask is None:
                continue
            t = self.tick_size(symbol)
            pos = self.position(symbol)
            skew = round(pos / 10) * t            # 1 tick per 10 units of inventory
            my_bid = self.snap(symbol, bid + t - skew)
            my_ask = self.snap(symbol, ask - t - skew)
            if my_ask - my_bid < 2 * t:           # spread too tight to improve: join instead
                my_bid, my_ask = self.snap(symbol, bid - skew), self.snap(symbol, ask - skew)
            if pos < MAX_POS:
                actions.append(Buy(symbol, my_bid, qty=SIZE))
            if pos > -MAX_POS:
                actions.append(Sell(symbol, my_ask, qty=SIZE))
        self.batch(actions)
        a = self.account
        self.log(f"pnl {a.pnl:+.2f}  margin used {a.margin_used:.0f}  pos {self.positions}")


MarketMaker().run(tick_ms=500)
