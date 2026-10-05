"""
Bot SDK for the Mini Exchange.

Minimal example
---------------

    from exchange import Exchange

    class MyBot(Exchange):
        def on_tick(self):
            fair = self.fair_value("etf")          # sum of the fruit mids
            ask = self.best_ask("etf")
            if fair and ask and ask < fair - 0.10:
                self.buy("etf", ask, qty=1, ioc=True)

    MyBot().run()

Market-maker example
--------------------

    from exchange import Exchange

    class MM(Exchange):
        def on_tick(self):
            actions = [CancelAll()]
            for symbol in self.fruits:
                mid = self.mid(symbol)
                if mid is None:
                    continue
                actions += [
                    Buy(symbol, self.snap(symbol, mid - 0.03), qty=5),
                    Sell(symbol, self.snap(symbol, mid + 0.03), qty=5),
                ]
            self.batch(actions)

    MM().run()

Run with:  python my_bot.py -u myname -p mypass [--url http://host:3211]
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import threading
import time
from dataclasses import dataclass, field
from typing import Optional, Union

try:
    import requests
    import websockets.sync.client as ws_client
except ImportError:  # the backtest needs neither
    requests = ws_client = None


DEFAULT_URL = "https://exchange.qfinuwa.org"

# Set by the exchange when it runs an uploaded bot in the backtest. The bot then talks to the
# simulated exchange over two pipes instead of HTTP/WebSocket; nothing else changes.
BACKTEST = os.environ.get("QFIN_BACKTEST") == "1"

# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------

@dataclass
class OrderResult:
    """Returned by buy() and sell()."""
    order_id: str = ""
    status: str = ""
    fills: list[Fill] = field(default_factory=list)

    @property
    def filled_qty(self) -> int:
        return sum(f.qty for f in self.fills)

    @property
    def avg_price(self) -> float:
        total = sum(f.price * f.qty for f in self.fills)
        qty = self.filled_qty
        return total / qty if qty else 0.0


@dataclass
class Fill:
    price: float = 0.0
    qty: int = 0


@dataclass
class BookLevel:
    price: float
    qty: int


@dataclass
class OrderBook:
    bids: list[BookLevel] = field(default_factory=list)
    asks: list[BookLevel] = field(default_factory=list)
    last_trade_price: Optional[float] = None


@dataclass
class OpenOrder:
    id: str
    symbol: str
    side: str
    price: float
    qty: int
    filled_qty: int
    status: str
    order_type: str

    @property
    def remaining(self) -> int:
        return self.qty - self.filled_qty


@dataclass
class EtfComponent:
    symbol: str
    weight: float


@dataclass
class Account:
    """Your account for the current trading day."""
    cash: float = 0.0
    equity: float = 0.0             # cash + positions marked at mid
    pnl: float = 0.0                # today's P&L (includes losses from busts)
    starting_cash: float = 0.0
    margin_used: float = 0.0
    margin_available: float = 0.0   # orders are rejected if they need more than this
    busts: int = 0                  # times you hit 0 equity and were reset today
    day: str = ""
    day_end: float = 0.0            # epoch seconds when the trading day ends


@dataclass
class OptionInfo:
    """The live window behind an up-down option symbol (up5, up60, ...)."""
    symbol: str
    expiry_min: int
    window_open: int                # epoch seconds
    window_close: int               # epoch seconds
    strike: Optional[float]         # ETF price at window open (None until known)
    price_source: str               # "etf_last_trade" or "etf_mid"
    seconds_left: float = 0.0


@dataclass
class OptionWindow:
    """A settled option window."""
    expiry_min: int
    window_open: int
    strike: float
    settle: float
    up_won: bool
    price_source: str


# ---------------------------------------------------------------------------
# Batch action helpers -- pass these to self.batch([...])
# ---------------------------------------------------------------------------

def Buy(symbol: str, price: float, qty: int, *, ioc: bool = False) -> dict:
    """Batch action: place a buy order."""
    return {"type": "order", "symbol": symbol, "side": "buy",
            "price": price, "qty": qty, "orderType": "ioc" if ioc else "limit"}

def Sell(symbol: str, price: float, qty: int, *, ioc: bool = False) -> dict:
    """Batch action: place a sell order."""
    return {"type": "order", "symbol": symbol, "side": "sell",
            "price": price, "qty": qty, "orderType": "ioc" if ioc else "limit"}

def CancelAll(symbol: str = "") -> dict:
    """Batch action: cancel all open orders (optionally for one symbol)."""
    d: dict = {"type": "cancelAll"}
    if symbol:
        d["symbol"] = symbol
    return d

def Cancel(order_id: str) -> dict:
    """Batch action: cancel a specific order."""
    return {"type": "cancel", "orderId": order_id}


# ---------------------------------------------------------------------------
# Exchange base class -- subclass this and override on_tick()
# ---------------------------------------------------------------------------

class _BacktestResponse:
    """Just enough of requests.Response for the SDK."""

    def __init__(self, status: int, body) -> None:
        self.status_code = status
        self.ok = 200 <= status < 300
        self._body = body
        self.text = body if isinstance(body, str) else json.dumps(body)

    def json(self):
        return self._body

    def raise_for_status(self) -> None:
        if not self.ok:
            raise RuntimeError(f"HTTP {self.status_code}: {self.text}")


class Exchange:
    """Base class for trading bots.

    Subclass and override ``on_tick()``.  Call ``run()`` to start.
    Market data is available via properties and methods below.
    """

    def __init__(self) -> None:
        self._base_url: str = ""
        self._ws_url: str = ""
        self._session_token: str = ""
        self._state: dict = {}
        self._tick_ms: int = 1000
        self._state_lock = threading.Lock()
        self._state_version: int = 0
        self._state_ready = threading.Event()
        self._state_updated_at: float = 0.0
        # keep-alive: much faster than a new connection per order
        self._http = requests.Session() if requests and not BACKTEST else None

    # ------------------------------------------------------------------
    # Override this
    # ------------------------------------------------------------------

    def on_tick(self) -> None:
        """Called every tick. Override this with your bot logic."""
        raise NotImplementedError("Override on_tick() in your bot class")

    # ------------------------------------------------------------------
    # Market data (read-only, updated automatically via WebSocket)
    # ------------------------------------------------------------------

    @property
    def products(self) -> list[str]:
        """All tradeable symbols.  Example: ["apples", ..., "etf", "up5", "up60"]"""
        return [p["symbol"] for p in self._state.get("products", [])]

    @property
    def fruits(self) -> list[str]:
        """The five fruits: apples, bananas, oranges, strawberries, watermelon."""
        return [p["symbol"] for p in self._state.get("products", []) if self._kind(p) == "fruit"]

    @property
    def etfs(self) -> list[str]:
        """ETF symbols only.  Example: ["etf"]"""
        return [p["symbol"] for p in self._state.get("products", []) if p.get("isEtf")]

    @property
    def options(self) -> list[str]:
        """Up-down option symbols that are currently enabled.  Example: ["up5", "up60"]"""
        return [p["symbol"] for p in self._state.get("products", []) if self._kind(p) == "option"]

    def kind(self, symbol: str) -> str:
        """"fruit", "etf" or "option" ("" if unknown)."""
        p = self._product(symbol)
        return self._kind(p) if p else ""

    def mid(self, symbol: str, default: Optional[float] = None) -> Optional[float]:
        """Mid-market price.

        Falls back to last trade price, then to ``default``.
        With no default, returns None when no data exists.

            mid = self.mid("apples")         # None if no quotes
            mid = self.mid("apples", 10.0)   # 10.0 if no quotes
        """
        book = self._state.get("books", {}).get(symbol)
        if book:
            bids = book.get("bids", [])
            asks = book.get("asks", [])
            if bids and asks:
                return (bids[0]["price"] + asks[0]["price"]) / 2
            if book.get("lastTradePrice") is not None:
                return book["lastTradePrice"]
        return default

    def best_bid(self, symbol: str) -> Optional[float]:
        """Highest bid price, or None."""
        bids = self._state.get("books", {}).get(symbol, {}).get("bids", [])
        return bids[0]["price"] if bids else None

    def best_ask(self, symbol: str) -> Optional[float]:
        """Lowest ask price, or None."""
        asks = self._state.get("books", {}).get(symbol, {}).get("asks", [])
        return asks[0]["price"] if asks else None

    def best_bid_qty(self, symbol: str) -> int:
        """Quantity at the best bid, or 0."""
        bids = self._state.get("books", {}).get(symbol, {}).get("bids", [])
        return bids[0]["qty"] if bids else 0

    def best_ask_qty(self, symbol: str) -> int:
        """Quantity at the best ask, or 0."""
        asks = self._state.get("books", {}).get(symbol, {}).get("asks", [])
        return asks[0]["qty"] if asks else 0

    def spread(self, symbol: str) -> Optional[float]:
        """Ask minus bid, or None if either side is empty."""
        bid = self.best_bid(symbol)
        ask = self.best_ask(symbol)
        if bid is not None and ask is not None:
            return ask - bid
        return None

    def book(self, symbol: str) -> OrderBook:
        """Full order book for a symbol."""
        raw = self._state.get("books", {}).get(symbol, {})
        return OrderBook(
            bids=[BookLevel(l["price"], l["qty"]) for l in raw.get("bids", [])],
            asks=[BookLevel(l["price"], l["qty"]) for l in raw.get("asks", [])],
            last_trade_price=raw.get("lastTradePrice"),
        )

    def last_trade(self, symbol: str) -> Optional[float]:
        """Last trade price, or None."""
        return self._state.get("books", {}).get(symbol, {}).get("lastTradePrice")

    def fair_value(self, symbol: str) -> Optional[float]:
        """Naive fair value: mid for fruits/options, sum of component mids for the ETF.

        Returns None if any required mid is missing.
        """
        if not self.is_etf(symbol):
            return self.mid(symbol)
        comp = self.etf_composition(symbol)
        if not comp:
            return self.mid(symbol)
        total = 0.0
        for c in comp:
            m = self.mid(c.symbol)
            if m is None:
                return None
            total += c.weight * m
        return total

    def tick_size(self, symbol: str) -> float:
        """Minimum price increment for this product (0.01 for everything)."""
        p = self._product(symbol)
        return p.get("tickSize", 0.01) if p else 0.01

    def snap(self, symbol: str, price: float) -> float:
        """Round a price to the nearest valid tick.  Option prices are also clamped to [0.01, 0.99]."""
        ts = self.tick_size(symbol)
        px = round(round(price / ts) * ts, 10)
        if self.kind(symbol) == "option":
            px = min(0.99, max(0.01, px))
        return px

    def position_limit(self, symbol: str) -> int:
        """Max absolute position for this product."""
        p = self._product(symbol)
        return int(p.get("positionLimit", 0)) if p else 0

    # ------------------------------------------------------------------
    # Up-down options
    # ------------------------------------------------------------------

    def option_info(self, symbol: str) -> Optional[OptionInfo]:
        """The current window for an option symbol, or None.

            info = self.option_info("up5")
            if info and info.strike is not None:
                etf_now = self.last_trade("etf")
                print(info.strike, info.seconds_left)

        "up" pays $1 if the ETF price at window_close >= strike (a tie is up), else $0.
        Selling up at p is the same as buying down at 1 - p.
        """
        p = self._product(symbol)
        o = p.get("option") if p else None
        if not o:
            return None
        return OptionInfo(
            symbol=symbol, expiry_min=o["expiryMin"], window_open=o["windowOpen"],
            window_close=o["windowClose"], strike=o.get("strike"),
            price_source=o.get("priceSource", ""),
            seconds_left=max(0.0, o["windowClose"] - self.server_time),
        )

    def option_windows(self, expiry_min: Optional[int] = None, limit: int = 100) -> list[OptionWindow]:
        """Recently settled windows, newest first (fetched over HTTP)."""
        params: dict = {"limit": limit}
        if expiry_min:
            params["expiry"] = expiry_min
        resp = self._get("/api/option-windows", params)
        if not resp.ok:
            return []
        return [
            OptionWindow(w["expiryMin"], w["windowOpen"], w["strike"], w["settle"],
                         w["upWon"], w["priceSource"])
            for w in resp.json()
        ]

    def is_etf(self, symbol: str) -> bool:
        """True if this symbol is an ETF."""
        for p in self._state.get("products", []):
            if p["symbol"] == symbol:
                return p.get("isEtf", False)
        return False

    def etf_compositions(self, symbol: str) -> list[list[EtfComponent]]:
        """All alternative compositions for an ETF.  Returns [] for non-ETFs."""
        for p in self._state.get("products", []):
            if p["symbol"] == symbol:
                raw = p.get("compositions") or []
                return [[EtfComponent(c["symbol"], c["weight"]) for c in comp] for comp in raw]
        return []

    def etf_composition(self, symbol: str, index: int = 0) -> list[EtfComponent]:
        """Single composition (first by default).  Returns [] for non-ETFs."""
        comps = self.etf_compositions(symbol)
        if index < len(comps):
            return comps[index]
        return []

    # ------------------------------------------------------------------
    # Your portfolio (read-only, updated automatically)
    # ------------------------------------------------------------------

    def position(self, symbol: str) -> int:
        """Your signed position.  Positive = long, negative = short, 0 = flat."""
        for p in self._state.get("positions", []):
            if p["symbol"] == symbol:
                return int(p["qty"])
        return 0

    @property
    def positions(self) -> dict[str, int]:
        """All positions as {symbol: qty}.  Only includes non-zero."""
        return {p["symbol"]: int(p["qty"]) for p in self._state.get("positions", []) if p["qty"] != 0}

    @property
    def pnl(self) -> float:
        """Today's P&L (realized + unrealized)."""
        return self._state.get("pnl", 0)

    @property
    def account(self) -> Account:
        """Cash, equity, margin and busts for today.

        Every trading day starts with ``starting_cash``.  Orders are rejected if the
        margin they need is more than your equity.  If equity hits 0 you are reset
        (a "bust"), and the loss still counts towards today's P&L.
        """
        a = self._state.get("account") or {}
        return Account(
            cash=a.get("cash", 0.0), equity=a.get("equity", 0.0), pnl=a.get("pnl", 0.0),
            starting_cash=a.get("startingCash", 0.0), margin_used=a.get("marginUsed", 0.0),
            margin_available=a.get("marginAvailable", 0.0), busts=a.get("busts", 0),
            day=a.get("day", ""), day_end=a.get("dayEnd", 0) / 1000,
        )

    @property
    def config(self) -> dict:
        """Exchange rules: phase, startingCash, dayLengthMin, etfFee, marginRates, enabledExpiries."""
        return dict(self._state.get("config") or {})

    @property
    def server_time(self) -> float:
        """Exchange clock in epoch seconds (from the latest state update)."""
        ms = self._state.get("serverTime")
        if not ms:
            return time.time()
        if BACKTEST:
            return ms / 1000
        return ms / 1000 + (time.time() - self._state_updated_at)

    def open_orders(self, symbol: Optional[str] = None) -> list[OpenOrder]:
        """Your resting orders, optionally filtered by symbol."""
        raw = self._state.get("openOrders", [])
        if symbol:
            raw = [o for o in raw if o["symbol"] == symbol]
        return [
            OpenOrder(
                id=o["_id"], symbol=o["symbol"], side=o["side"],
                price=o["price"], qty=o["qty"], filled_qty=o["filledQty"],
                status=o["status"], order_type=o["orderType"],
            )
            for o in raw
        ]

    # ------------------------------------------------------------------
    # Trading actions
    # ------------------------------------------------------------------

    def buy(self, symbol: str, price: float, qty: int, *, ioc: bool = False) -> OrderResult:
        """Place a buy order.

        Args:
            symbol: e.g. "apples", "etf", "up5"
            price:  limit price
            qty:    number of units
            ioc:    if True, unfilled portion is cancelled immediately
        """
        return self._place("buy", symbol, price, qty, ioc)

    def sell(self, symbol: str, price: float, qty: int, *, ioc: bool = False) -> OrderResult:
        """Place a sell order.  Same args as buy()."""
        return self._place("sell", symbol, price, qty, ioc)

    def cancel(self, order_id: str) -> bool:
        """Cancel a specific order.  Returns True on success."""
        resp = self._post("/api/order", method="DELETE", json={"orderId": order_id})
        return resp.ok

    def cancel_all(self, symbol: Optional[str] = None) -> bool:
        """Cancel all your open orders.  Pass symbol to cancel only that product."""
        body: dict = {}
        if symbol:
            body["symbol"] = symbol
        resp = self._post("/api/cancel-all", json=body)
        return resp.ok

    def batch(self, actions: list[dict]) -> list[dict]:
        """Execute multiple actions atomically in one round trip.

        Use the helper functions Buy(), Sell(), CancelAll(), Cancel()::

            self.batch([
                CancelAll(),
                Buy("apples", 10.95, qty=5),
                Sell("apples", 11.05, qty=5),
            ])
        """
        resolved = []
        for a in actions:
            a = dict(a)
            if "symbol" in a:
                pid = self._symbol_to_id(a["symbol"])
                if pid:
                    a["productId"] = pid
                    del a["symbol"]
            resolved.append(a)
        t0 = time.time()
        resp = self._post("/api/batch", json={"actions": resolved})
        dt = int((time.time() - t0) * 1000)
        if not resp.ok:
            self.warn(f"batch failed ({dt}ms): {self._err(resp)}")
            return []
        return resp.json()

    # ------------------------------------------------------------------
    # ETF creation / redemption
    # ------------------------------------------------------------------

    def create_etf(self, symbol: str, qty: int, *, comp_idx: int = 0) -> dict:
        """Turn 1 of each fruit into 1 ETF, qty times.  Costs config["etfFee"] per unit.

        Returns {"qty": units created, "fee": total fee, ...}.
        """
        return self._etf_swap(symbol, qty, "create", comp_idx=comp_idx)

    def redeem_etf(self, symbol: str, qty: int, *, comp_idx: int = 0) -> dict:
        """Turn 1 ETF back into 1 of each fruit, qty times.  Costs config["etfFee"] per unit."""
        return self._etf_swap(symbol, qty, "redeem", comp_idx=comp_idx)

    # ------------------------------------------------------------------
    # Historical data
    # ------------------------------------------------------------------

    def download_data(self, dest: str = "data", *, overwrite_today: bool = True) -> list[str]:
        """Download the exchange's recorded Parquet files into ``dest``.

        Layout matches the handout: dest/<table>/date=YYYY-MM-DD.parquet.
        Finished days are skipped if already downloaded.  Returns the paths written.
        Works before run() too: ``Exchange().connect(...).download_data()``.
        """
        if BACKTEST:
            self.warn("download_data() does nothing in the backtest")
            return []
        resp = self._http.get(f"{self._base_url}/api/data")
        resp.raise_for_status()
        files = resp.json()
        today = time.strftime("%Y-%m-%d", time.gmtime())
        written = []
        for f in files:
            out = os.path.join(dest, f["path"])
            if os.path.exists(out) and not (overwrite_today and f["date"] == today):
                continue
            os.makedirs(os.path.dirname(out), exist_ok=True)
            r = self._http.get(f"{self._base_url}/api/data/{f['path']}")
            r.raise_for_status()
            with open(out, "wb") as fh:
                fh.write(r.content)
            written.append(out)
        return written

    # ------------------------------------------------------------------
    # Logging (use these in your bot)
    # ------------------------------------------------------------------

    def log(self, msg: str) -> None:
        """Print a green [OK] message."""
        print(f"\033[32m[OK]\033[0m {msg}")

    def warn(self, msg: str) -> None:
        """Print a yellow [!] warning."""
        print(f"\033[33m[!]\033[0m  {msg}", file=sys.stderr)

    # ------------------------------------------------------------------
    # Run loop (call this once)
    # ------------------------------------------------------------------

    def run(self, tick_ms: int = 1000) -> None:
        """Log in, connect to the exchange, and start the tick loop.

        CLI args are parsed automatically::

            -u / --username     (default: "bot")
            -p / --password     (default: same as username)
            --url               exchange URL (default: $EXCHANGE_URL or the QFin server)
            --tick              tick interval in ms
        """
        parser = argparse.ArgumentParser()
        parser.add_argument("--username", "-u", default="bot")
        parser.add_argument("--password", "-p", default=None)
        parser.add_argument("--url", default=None)
        parser.add_argument("--tick", type=int, default=None)
        cli, _ = parser.parse_known_args()

        if BACKTEST:
            self._backtest_loop()
            return

        if cli.tick is not None:
            tick_ms = cli.tick
        self._tick_ms = tick_ms

        self.connect(cli.username, cli.password or cli.username, url=cli.url)

        threading.Thread(target=self._ws_loop, daemon=True).start()

        self.log("waiting for market data...")
        self._state_ready.wait()
        self.log(f"ready - {len(self.products)} products")

        while True:
            t = time.time()
            try:
                self.on_tick()
            except KeyboardInterrupt:
                self.log("stopped")
                break
            except Exception as e:
                self.warn(str(e))
            wait = (self._tick_ms / 1000) - (time.time() - t)
            if wait > 0:
                time.sleep(wait)

    def connect(self, username: str, password: str, *, url: Optional[str] = None) -> "Exchange":
        """Log in without starting the tick loop (run() calls this for you)."""
        if BACKTEST:
            return self
        if requests is None:
            self._die("pip install requests websockets")
        self._base_url = (url or os.environ.get("EXCHANGE_URL") or DEFAULT_URL).rstrip("/")
        self._ws_url = re.sub(r"^http", "ws", self._base_url) + "/ws"
        resp = self._http.post(f"{self._base_url}/api/auth", json={"username": username, "password": password})
        if not resp.ok:
            self._die(f"Login failed: {self._err(resp)}")
        data = resp.json()
        self._session_token = data["token"]
        self._http.headers["Authorization"] = f"Bearer {self._session_token}"
        self.log(f"logged in as {data['username']} on {self._base_url}")
        return self

    # ==================================================================
    # Private helpers (you don't need to touch anything below)
    # ==================================================================

    def _product(self, symbol: str) -> Optional[dict]:
        for p in self._state.get("products", []):
            if p["symbol"] == symbol:
                return p
        return None

    @staticmethod
    def _kind(p: dict) -> str:
        return p.get("kind") or ("etf" if p.get("isEtf") else "fruit")

    # Keep old _log/_warn as aliases so existing bots don't break
    def _log(self, msg: str) -> None:
        self.log(msg)

    def _warn(self, msg: str) -> None:
        self.warn(msg)

    def _place(self, side: str, symbol: str, price: float, qty: int, ioc: bool) -> OrderResult:
        resp = self._post("/api/order", json={
            "symbol": symbol, "side": side,
            "price": price, "qty": qty,
            "orderType": "ioc" if ioc else "limit",
        })
        if not resp.ok:
            self.warn(f"{side.upper()} {qty} {symbol} @ {price:.2f} rejected: {self._err(resp)}")
            return OrderResult()
        d = resp.json()
        return OrderResult(
            order_id=d.get("orderId", ""),
            status=d.get("status", ""),
            fills=[Fill(f["price"], f["qty"]) for f in (d.get("fills") or [])],
        )

    def _etf_swap(self, symbol: str, qty: int, direction: str, *, comp_idx: int = 0) -> dict:
        comp = self.etf_composition(symbol, comp_idx)
        prices = [self.mid(c.symbol) for c in comp]
        body: dict = {"symbol": symbol, "qty": qty, "direction": direction, "compositionIndex": comp_idx}
        if all(p is not None for p in prices):
            body["prices"] = prices
        resp = self._post("/api/etf-swap", json=body)
        if not resp.ok:
            self.warn(f"ETF {direction} {qty} {symbol}: {self._err(resp)}")
            return {}
        return resp.json()

    def _post(self, path: str, *, method: str = "POST", json: dict = None) -> requests.Response:
        if BACKTEST:
            return self._bt_call(method, path, json)
        return self._http.request(method, f"{self._base_url}{path}", json=json)

    def _get(self, path: str, params: Optional[dict] = None) -> requests.Response:
        if BACKTEST:
            from urllib.parse import urlencode
            return self._bt_call("GET", path + ("?" + urlencode(params) if params else ""), None)
        return self._http.get(f"{self._base_url}{path}", params=params)

    # --- backtest transport (fd 3: from the exchange, fd 4: to the exchange) ---

    def _backtest_loop(self) -> None:
        import traceback
        self._bt_in = os.fdopen(3, "r")
        self._bt_out = os.fdopen(4, "w")
        self._bt_send({"type": "ready"})
        errors = 0
        while True:
            line = self._bt_in.readline()
            if not line:
                return
            msg = json.loads(line)
            if msg["type"] == "end":
                return
            if msg["type"] != "tick":
                continue
            self._state = msg["state"]
            self._state_version += 1
            self._state_updated_at = time.time()
            self._state_ready.set()
            try:
                self.on_tick()
            except Exception:
                errors += 1
                if errors <= 20:
                    traceback.print_exc()
                elif errors == 21:
                    self.warn("more on_tick errors, not printing them")
            self._bt_send({"type": "done"})

    def _bt_send(self, msg: dict) -> None:
        self._bt_out.write(json.dumps(msg) + "\n")
        self._bt_out.flush()

    def _bt_call(self, method: str, path: str, body: Optional[dict]) -> "_BacktestResponse":
        self._bt_send({"type": "call", "method": method, "path": path, "body": body})
        line = self._bt_in.readline()
        if not line:
            sys.exit(0)
        msg = json.loads(line)
        return _BacktestResponse(msg.get("status", 500), msg.get("body"))

    def _symbol_to_id(self, symbol: str) -> Optional[str]:
        for p in self._state.get("products", []):
            if p["symbol"] == symbol:
                return p.get("id")
        return None

    def _ws_loop(self) -> None:
        while True:
            try:
                with ws_client.connect(self._ws_url) as ws:
                    ws.send(json.dumps({"type": "auth", "token": self._session_token}))
                    for raw in ws:
                        msg = json.loads(raw)
                        if msg.get("type") == "state":
                            with self._state_lock:
                                self._state = msg["data"]
                                self._state_version += 1
                                self._state_updated_at = time.time()
                            self._state_ready.set()
                        elif msg.get("type") == "error":
                            self.warn(f"WS error: {msg.get('error')}")
                            break
            except Exception as e:
                self.warn(f"WS disconnected: {e}")
            time.sleep(1)

    def _err(self, resp: requests.Response) -> str:
        try:
            return resp.json().get("error", resp.text)
        except Exception:
            return resp.text

    def _die(self, msg: str) -> None:
        print(f"\033[31m[X]\033[0m  {msg}", file=sys.stderr)
        sys.exit(1)
