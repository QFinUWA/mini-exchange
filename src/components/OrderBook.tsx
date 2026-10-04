"use client";

import { useRef, useState, useCallback, useEffect } from "react";
import { createPortal } from "react-dom";
import {
  useOrderBook,
  useOpenOrders,
  usePositions,
  useCancelAll,
  usePlaceOrder,
  useProduct,
  useConfig,
  useBasketValue,
  useNowSec,
  formatPrice,
  formatDuration,
  errorMessage,
  type OptionInfo,
} from "@/lib/exchange";

interface Props {
  symbol: string;
  compositions?: Array<Array<{ symbol: string; weight: number }>>;
}

function useClickQty(symbol: string): [number, (v: number) => void] {
  const key = `click_qty_${symbol}`;
  const [qty, setQty] = useState(() => {
    if (typeof window === "undefined") return 10;
    const stored = localStorage.getItem(key);
    return stored ? parseInt(stored, 10) || 10 : 10;
  });
  const set = useCallback(
    (v: number) => {
      const clamped = Math.max(1, Math.floor(v));
      setQty(clamped);
      localStorage.setItem(key, String(clamped));
    },
    [key],
  );
  return [qty, set];
}

export function OrderBook({ symbol, compositions }: Props) {
  const book = useOrderBook(symbol);
  const product = useProduct(symbol);
  const clickTrading = useConfig()?.clickTrading ?? true;
  const tick = product?.tickSize ?? 0.01;
  const fmt = (p: number | null | undefined) => formatPrice(p, tick);
  const allOrders = useOpenOrders();
  const positions = usePositions();
  const cancelAll = useCancelAll();
  const placeOrder = usePlaceOrder();
  const [cancelling, setCancelling] = useState(false);
  const [clickQty, setClickQty] = useClickQty(symbol);

  const spreadRef = useRef<HTMLDivElement>(null);

  const userQtyByLevel = new Map<string, number>();
  let myOrderCount = 0;
  let myBidVol = 0;
  let myAskVol = 0;
  for (const o of allOrders) {
    if (o.symbol === symbol && (o.status === "open" || o.status === "partial")) {
      const rest = o.qty - o.filledQty;
      const key = `${o.side}:${o.price}`;
      userQtyByLevel.set(key, (userQtyByLevel.get(key) ?? 0) + rest);
      myOrderCount++;
      if (o.side === "buy") myBidVol += rest;
      else myAskVol += rest;
    }
  }

  const myPos = positions.find((p) => p.symbol === symbol)?.qty ?? 0;

  const asks = book?.asks ? [...book.asks].sort((a, b) => b.price - a.price) : [];
  const bids = book?.bids ? [...book.bids].sort((a, b) => b.price - a.price) : [];

  const bestAsk = asks.length ? asks[asks.length - 1].price : null;
  const bestBid = bids.length ? bids[0].price : null;
  const mid = bestAsk !== null && bestBid !== null ? (bestAsk + bestBid) / 2 : null;

  const handleCancelAll = async () => {
    setCancelling(true);
    try {
      await cancelAll(symbol);
    } catch {}
    setCancelling(false);
  };

  const [flash, setFlash] = useState<{ msg: string; error: boolean } | null>(null);

  const handleClickTrade = (price: number, side: "buy" | "sell") => {
    if (!clickTrading) return;
    const depthRows = side === "buy"
      ? asks.filter((r) => r.price <= price)
      : bids.filter((r) => r.price >= price);
    const depthQty = depthRows.reduce((s, r) => s + r.qty, 0);
    const qty = Math.min(clickQty, depthQty);
    if (qty <= 0) return;

    placeOrder(symbol, side, price, qty, "ioc")
      .then((res) => {
        const filledQty = res.fills?.reduce((s, f) => s + f.qty, 0) ?? 0;
        console.log(`[CLICK] ${symbol} ${side} ${qty}@${price} => ${res.status}, filled=${filledQty}/${qty}`);
      })
      .catch((err) => {
        const msg = errorMessage(err, "Order failed");
        setFlash({ msg, error: true });
        setTimeout(() => setFlash(null), 3000);
      });
  };

  return (
    <div className="flex-1 min-w-[210px] bg-zinc-900/50 border border-zinc-800/60 rounded-xl flex flex-col h-full overflow-hidden relative">
      <div className="px-3 py-2 border-b border-zinc-800/60 h-[84px] shrink-0 overflow-hidden whitespace-nowrap">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 min-w-0">
            <span className="text-sm font-bold text-white shrink-0">{symbol}</span>
            {product?.kind === "option" && (
              <span className="text-[10px] text-sky-400/80 bg-sky-500/10 border border-sky-500/20 rounded px-1.5 py-0.5 shrink-0">
                UP {product.option?.expiryMin}m
              </span>
            )}
            {compositions && compositions.length > 0 && (
              <EtfBadge compositions={compositions} />
            )}
            {myPos !== 0 && (
              <span className={`text-xs font-medium tabular-nums shrink-0 ${myPos > 0 ? "text-emerald-400/70" : "text-red-400/70"}`}>
                {myPos > 0 ? "+" : ""}{myPos}
              </span>
            )}
          </div>
          {myOrderCount > 0 && (
            <button
              onClick={handleCancelAll}
              disabled={cancelling}
              className="px-2 py-0.5 text-[10px] font-medium text-red-400/80 rounded-md hover:bg-red-500/10 disabled:opacity-30 transition-all shrink-0"
            >
              {cancelling ? "..." : `Cancel ${myOrderCount}`}
            </button>
          )}
        </div>
        <div className="flex items-center justify-between mt-1">
          <div className="flex items-center gap-2.5 text-xs tabular-nums text-zinc-500 min-w-0">
            <span>
              Last <span className="text-zinc-300 font-medium">{fmt(book?.lastTradePrice)}</span>
            </span>
            <span>
              Mid <span className="text-zinc-300 font-medium">{fmt(mid)}</span>
            </span>
            {(myBidVol > 0 || myAskVol > 0) && (
              <span>
                <span className="text-emerald-400/60">{myBidVol}</span>
                <span className="text-zinc-600">/</span>
                <span className="text-red-400/60">{myAskVol}</span>
              </span>
            )}
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            {clickTrading ? (
            <input
              type="number"
              min={1}
              title="Quantity sent when you click a price level"
              value={clickQty}
              onChange={(e) => setClickQty(parseInt(e.target.value, 10) || 1)}
              className="w-12 px-1 py-0.5 text-xs tabular-nums text-center bg-zinc-800 border border-zinc-700/60 rounded-md text-zinc-200 focus:outline-none focus:border-zinc-500"
            />
            ) : (
              <span className="text-[10px] text-zinc-600" title="Click trading is disabled by the admin">view only</span>
            )}
          </div>
        </div>
        {product?.kind === "option" && product.option && <OptionLine option={product.option} />}
        {product?.kind === "etf" && <BasketLine symbol={symbol} mid={mid} />}
      </div>

      <div className="flex justify-center px-3 py-1.5 text-[10px] text-zinc-600 border-b border-zinc-800/40 gap-2">
        <span className="w-8 text-right text-emerald-500/50">Bid</span>
        <span className="w-8 text-right text-emerald-500/30">You</span>
        <span className="w-16 text-center">Price</span>
        <span className="w-8 text-left text-red-400/30">You</span>
        <span className="w-8 text-left text-red-400/50">Ask</span>
      </div>

      {book === null ? (
        <div className="flex-1 flex items-center justify-center text-xs text-zinc-600">
          Loading...
        </div>
      ) : asks.length === 0 && bids.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-xs text-zinc-600">
          Empty book
        </div>
      ) : (
        <>
          <div className="flex-1 min-h-0 overflow-y-auto flex flex-col justify-end text-xs tabular-nums">
            {asks.map((row) => {
              const myQty = userQtyByLevel.get(`sell:${row.price}`) ?? 0;
              return (
                <div
                  key={`a-${row.price}`}
                  onClick={() => handleClickTrade(row.price, "buy")}
                  className={`flex justify-center gap-2 px-3 py-[3px] ${clickTrading ? "cursor-pointer hover:bg-red-500/10" : ""} ${
                    myQty > 0 ? "bg-red-500/8 border-l-2 border-l-red-400/40" : "border-l-2 border-l-transparent"
                  }`}
                >
                  <span className="w-8" />
                  <span className="w-8" />
                  <span className="w-16 text-center text-red-400/80">{fmt(row.price)}</span>
                  <span className="w-8 text-left text-red-400/30">{myQty > 0 ? myQty : ""}</span>
                  <span className="w-8 text-left text-red-400/60">{row.qty}</span>
                </div>
              );
            })}
          </div>

          <div
            ref={spreadRef}
            className="h-[3px] bg-zinc-700/60 shrink-0"
          />

          <div className="flex-1 min-h-0 overflow-y-auto text-xs tabular-nums">
            {bids.map((row) => {
              const myQty = userQtyByLevel.get(`buy:${row.price}`) ?? 0;
              return (
                <div
                  key={`b-${row.price}`}
                  onClick={() => handleClickTrade(row.price, "sell")}
                  className={`flex justify-center gap-2 px-3 py-[3px] ${clickTrading ? "cursor-pointer hover:bg-emerald-500/10" : ""} ${
                    myQty > 0 ? "bg-emerald-500/8 border-l-2 border-l-emerald-400/40" : "border-l-2 border-l-transparent"
                  }`}
                >
                  <span className="w-8 text-right text-emerald-400/60">{row.qty}</span>
                  <span className="w-8 text-right text-emerald-400/30">{myQty > 0 ? myQty : ""}</span>
                  <span className="w-16 text-center text-emerald-400/80">{fmt(row.price)}</span>
                  <span className="w-8" />
                  <span className="w-8" />
                </div>
              );
            })}
          </div>
        </>
      )}
      {flash && (
        <div className={`absolute bottom-0 left-0 right-0 px-3 py-1.5 text-[10px] truncate ${
          flash.error
            ? "text-red-400 bg-red-500/10"
            : "text-zinc-400 bg-zinc-800/90"
        }`}>
          {flash.msg}
        </div>
      )}
    </div>
  );
}

function OptionLine({ option }: { option: OptionInfo }) {
  const etf = useOrderBook("etf");
  const now = useNowSec();
  const left = option.windowClose - now;
  const etfPx = etf?.lastTradePrice ?? null;
  const diff = etfPx !== null && option.strike !== null ? etfPx - option.strike : null;
  return (
    <div
      className="flex items-center gap-2.5 mt-1.5 text-[11px] tabular-nums text-zinc-500"
      title={`Pays $1 if the ETF ${option.priceSource === "etf_mid" ? "mid" : "last trade"} at window close >= strike`}
    >
      <span>K <span className="text-zinc-300">{formatPrice(option.strike)}</span></span>
      <span>
        ETF <span className="text-zinc-300">{formatPrice(etfPx)}</span>
        {diff !== null && (
          <span className={diff >= 0 ? "text-emerald-400/80" : "text-red-400/80"}> {diff >= 0 ? "+" : ""}{diff.toFixed(2)}</span>
        )}
      </span>
      <span className={`ml-auto ${left < 30 ? "text-amber-400" : "text-zinc-300"}`}>{formatDuration(left)}</span>
    </div>
  );
}

function BasketLine({ symbol, mid }: { symbol: string; mid: number | null }) {
  const basket = useBasketValue(symbol);
  const diff = basket !== null && mid !== null ? mid - basket : null;
  return (
    <div className="flex items-center gap-2.5 mt-1.5 text-[11px] tabular-nums text-zinc-500" title="Sum of the fruit mids">
      <span>Basket <span className="text-zinc-300">{formatPrice(basket)}</span></span>
      {diff !== null && (
        <span>
          Prem <span className={diff >= 0 ? "text-emerald-400/80" : "text-red-400/80"}>{diff >= 0 ? "+" : ""}{diff.toFixed(2)}</span>
        </span>
      )}
    </div>
  );
}

function EtfBadge({ compositions }: { compositions: Array<Array<{ symbol: string; weight: number }>> }) {
  const [open, setOpen] = useState(false);
  const btnRef = useRef<HTMLButtonElement>(null);
  const [pos, setPos] = useState({ top: 0, left: 0 });

  useEffect(() => {
    if (open && btnRef.current) {
      const rect = btnRef.current.getBoundingClientRect();
      setPos({ top: rect.bottom + 4, left: rect.left });
    }
  }, [open]);

  return (
    <span className="shrink-0">
      <button
        ref={btnRef}
        type="button"
        onClick={(e) => { e.stopPropagation(); setOpen((v) => !v); }}
        className="text-[10px] text-violet-400/70 bg-violet-500/10 border border-violet-500/20 rounded px-1.5 py-0.5 hover:bg-violet-500/20 transition-colors"
      >
        ETF
      </button>
      {open && createPortal(
        <>
          <div className="fixed inset-0 z-50" onClick={() => setOpen(false)} />
          <div
            className="fixed z-50 bg-zinc-900 border border-zinc-700/60 rounded-lg shadow-xl p-3 min-w-[140px]"
            style={{ top: pos.top, left: pos.left }}
          >
            {compositions.map((comp, ci) => (
              <div key={ci}>
                {compositions.length > 1 && (
                  <div className="text-[10px] text-zinc-500 mb-1 mt-1 first:mt-0">
                    {ci > 0 && <div className="border-t border-zinc-700/40 my-1.5" />}
                    Basket {ci + 1}
                  </div>
                )}
                {compositions.length === 1 && (
                  <div className="text-[10px] text-zinc-500 mb-1.5">Composition</div>
                )}
                {comp.map((c) => (
                  <div key={c.symbol} className="flex items-center justify-between gap-4 py-0.5">
                    <span className="text-xs text-zinc-200 font-medium">{c.symbol}</span>
                    <span className="text-xs text-zinc-400 tabular-nums">{c.weight}x</span>
                  </div>
                ))}
              </div>
            ))}
          </div>
        </>,
        document.body
      )}
    </span>
  );
}
