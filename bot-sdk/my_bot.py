"""
Your trading bot. Edit on_tick() with your strategy.

Run:  python my_bot.py -u YOUR_TEAM -p YOUR_PASSWORD
"""

from exchange import Exchange, Buy, Sell, CancelAll

class MyBot(Exchange):

    def on_tick(self):
        for symbol in self.fruits:
            mid = self.mid(symbol)
            if mid is None:
                continue

            pos = self.position(symbol)

            # --- Your strategy here ---

            self.log(f"{symbol}: mid={mid:.2f} pos={pos}")

        self.log(f"P&L today: {self.account.pnl:+.2f}")


MyBot().run()
