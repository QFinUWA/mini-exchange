"""
Example bot that uses every feature of the SDK.

THIS BOT IS INTENTIONALLY BAD. It does too many things at once,
has arbitrary thresholds, and will probably lose money. It exists
to show you what's available, not as a strategy to copy.

Your bot should do ONE thing well, not everything poorly.

Run:  python example_bot.py -u YOUR_TEAM -p YOUR_PASSWORD
"""

from exchange import Exchange, Buy, Sell, CancelAll


class KitchenSinkBot(Exchange):

    def on_tick(self):

        # ---- Account status ----
        acct = self.account
        self.log(f"P&L {acct.pnl:+.2f}  equity {acct.equity:.2f}  "
                 f"margin free {acct.margin_available:.2f}  positions {self.positions}")

        # ---- Market make every fruit ----
        # Cancel stale orders first, then requote. One batch = one round trip.
        actions = [CancelAll()]

        for symbol in self.fruits:
            mid = self.mid(symbol)            # None if the book is empty
            if mid is None:
                continue
            pos = self.position(symbol)

            # Lean away from risk: long -> quote lower, short -> quote higher
            skew = pos * 0.002

            bid = self.snap(symbol, mid - 0.04 - skew)
            ask = self.snap(symbol, mid + 0.04 - skew)

            if pos < 50:
                actions.append(Buy(symbol, bid, qty=3))
            if pos > -50:
                actions.append(Sell(symbol, ask, qty=3))

        self.batch(actions)

        # ---- ETF vs basket (badly) ----
        for symbol in self.etfs:
            fair = self.fair_value(symbol)    # sum of the fruit mids
            bid, ask = self.best_bid(symbol), self.best_ask(symbol)
            if fair is None or bid is None or ask is None:
                continue
            fee = self.config.get("etfFee", 0)

            if bid > fair + 0.25:
                self.sell(symbol, bid, min(2, self.best_bid_qty(symbol)), ioc=True)
                self.log(f"ETF rich: bid {bid:.2f} vs basket {fair:.2f}")
            elif ask < fair - 0.25:
                self.buy(symbol, ask, min(2, self.best_ask_qty(symbol)), ioc=True)
                self.log(f"ETF cheap: ask {ask:.2f} vs basket {fair:.2f}")

            # Turn ETF back into fruit (or fruit into ETF) for a fee per unit
            etf_pos = self.position(symbol)
            if etf_pos > 0:
                res = self.redeem_etf(symbol, etf_pos)
                self.log(f"redeemed {res.get('qty')} ETF, fee {res.get('fee')} (fee/unit {fee})")

        # ---- Up-down options ----
        for symbol in self.options:
            info = self.option_info(symbol)   # strike, window_close, seconds_left, ...
            etf_now = self.last_trade("etf")
            if info is None or info.strike is None or etf_now is None:
                continue
            ask = self.best_ask(symbol)
            # "Up" pays $1 if the ETF last trade at window close >= strike.
            # Silly rule: buy up cheaply near expiry if the ETF is well above the strike.
            if ask is not None and info.seconds_left < 30 and etf_now > info.strike + 0.2 and ask < 0.80:
                self.buy(symbol, ask, qty=5, ioc=True)
                self.log(f"{symbol}: ETF {etf_now:.2f} > strike {info.strike:.2f}, bought up @ {ask:.2f}")

        # ---- Panic flatten if losing too much ----
        if acct.pnl < -500:
            self.log("PANIC: P&L below -500, flattening everything")
            self.cancel_all()
            for symbol, qty in self.positions.items():
                if qty > 0:
                    bid = self.best_bid(symbol)
                    if bid: self.sell(symbol, bid, qty, ioc=True)
                elif qty < 0:
                    ask = self.best_ask(symbol)
                    if ask: self.buy(symbol, ask, abs(qty), ioc=True)

        # ---- Recent option results (HTTP call, so not every tick) ----
        if int(self.server_time) % 60 == 0:
            for w in self.option_windows(5, limit=3):
                self.log(f"up5 window {w.window_open}: strike {w.strike:.2f} settle {w.settle:.2f} "
                         f"-> {'UP' if w.up_won else 'DOWN'}")


KitchenSinkBot().run()
