"""
Demo bot 2: ETF arbitrage. Trades the ETF against the basket of fruit when the gap is
bigger than spreads + the create/redeem fee, then unwinds via create/redeem.

Run:  python etf_arb.py -u arb-demo -p <password> --url http://localhost:3211
"""
import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "bot-sdk"))
from exchange import Exchange

QTY = 3
EDGE = 0.04   # extra profit wanted per unit on top of fees


class EtfArb(Exchange):
    def on_tick(self):
        fee = self.config.get("etfFee", 0.05)
        fruits = self.fruits
        etf_bid, etf_ask = self.best_bid("etf"), self.best_ask("etf")
        bids = [self.best_bid(f) for f in fruits]
        asks = [self.best_ask(f) for f in fruits]
        if None in bids or None in asks or etf_bid is None or etf_ask is None:
            return
        basket_bid, basket_ask = sum(bids), sum(asks)

        # ETF rich: sell ETF, buy fruit, create ETF from the fruit to deliver
        if etf_bid - basket_ask > fee + EDGE:
            q = min(QTY, self.best_bid_qty("etf"), *[self.best_ask_qty(f) for f in fruits])
            if q > 0 and self.sell("etf", etf_bid, q, ioc=True).filled_qty:
                for f in fruits:
                    self.buy(f, self.best_ask(f), q, ioc=True)
                self.log(f"ETF rich by {etf_bid - basket_ask:.2f}: sold {q} ETF, bought basket")

        # ETF cheap: buy ETF, sell fruit, redeem ETF into fruit to deliver
        elif basket_bid - etf_ask > fee + EDGE:
            q = min(QTY, self.best_ask_qty("etf"), *[self.best_bid_qty(f) for f in fruits])
            if q > 0 and self.buy("etf", etf_ask, q, ioc=True).filled_qty:
                for f in fruits:
                    self.sell(f, self.best_bid(f), q, ioc=True)
                self.log(f"ETF cheap by {basket_bid - etf_ask:.2f}: bought {q} ETF, sold basket")

        # Net out matched ETF / fruit legs with create or redeem
        etf_pos = self.position("etf")
        fruit_pos = [self.position(f) for f in fruits]
        if etf_pos < 0 and min(fruit_pos) > 0:
            self.create_etf("etf", min(-etf_pos, min(fruit_pos)))
        elif etf_pos > 0 and max(fruit_pos) < 0:
            self.redeem_etf("etf", min(etf_pos, -max(fruit_pos)))

        self.log(f"gap rich {etf_bid - basket_ask:+.2f} cheap {basket_bid - etf_ask:+.2f}  "
                 f"pnl {self.account.pnl:+.2f}")


EtfArb().run(tick_ms=300)
