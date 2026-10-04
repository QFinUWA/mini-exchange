"use client";

import { useState, useRef, useEffect } from "react";
import { useRecentTrades, formatPrice } from "@/lib/exchange";

export function TradeHistory() {
  const [showMineOnly, setShowMineOnly] = useState(false);
  const trades = useRecentTrades();

  const seenIds = useRef(new Set<string>());
  const [newIds, setNewIds] = useState(new Set<string>());

  useEffect(() => {
    if (!trades.length) return;
    for (const t of trades) {
      if (!seenIds.current.has(t._id)) {
        seenIds.current.add(t._id);
        setNewIds((prev) => new Set(prev).add(t._id));
        setTimeout(() => {
          setNewIds((prev) => {
            const next = new Set(prev);
            next.delete(t._id);
            return next;
          });
        }, 2000);
      }
    }
  }, [trades]);

  const filtered = showMineOnly
    ? trades.filter((t) => t.isMine)
    : trades;

  const pill = (label: string, active: boolean, onClick: () => void) => (
    <button
      onClick={onClick}
      className={`px-2 py-0.5 text-[10px] font-medium rounded-md cursor-pointer transition-all ${
        active ? "bg-zinc-700 text-white" : "text-zinc-600 hover:text-zinc-300 hover:bg-zinc-800/50"
      }`}
    >
      {label}
    </button>
  );

  return (
    <section className="flex-1 min-h-0 flex flex-col">
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-zinc-800/60 shrink-0">
        <div className="flex items-center gap-2">
          <h2 className="text-xs font-semibold text-zinc-400">Trade History</h2>
          <span className="text-xs text-zinc-600">{filtered.length}</span>
        </div>
        <div className="flex items-center gap-1">
          {pill("All", !showMineOnly, () => setShowMineOnly(false))}
          {pill("Mine", showMineOnly, () => setShowMineOnly(true))}
        </div>
      </div>

      <div className="text-[11px] tabular-nums flex flex-col flex-1 min-h-0">
        <div className="grid grid-cols-[1fr_55px_55px_50px_35px] px-3 py-1.5 text-[10px] text-zinc-600 border-b border-zinc-800/40 shrink-0">
          <span>Symbol</span>
          <span>Bidder</span>
          <span>Asker</span>
          <span className="text-right">Price</span>
          <span className="text-right">Qty</span>
        </div>

        <div className="overflow-y-auto flex-1 min-h-0">
          {filtered.length === 0 ? (
            <div className="px-3 py-3 text-zinc-600 text-xs">
              {showMineOnly ? "No trades yet" : "No trades"}
            </div>
          ) : (
            filtered.map((t) => (
              <div
                key={t._id}
                className={`grid grid-cols-[1fr_55px_55px_50px_35px] px-3 py-1 border-b border-zinc-800/30 hover:bg-zinc-800/20 items-center transition-colors duration-1000 ${
                  newIds.has(t._id) ? "bg-zinc-600/25" : t.isMine ? "bg-zinc-800/10" : ""
                }`}
              >
                <span className="text-zinc-300 truncate font-medium">{t.symbol}</span>
                <span className={`truncate ${t.mySide === "buy" ? "text-emerald-400/70" : "text-zinc-500"}`}>
                  {t.buyer}
                </span>
                <span className={`truncate ${t.mySide === "sell" ? "text-red-400/70" : "text-zinc-500"}`}>
                  {t.seller}
                </span>
                <span className="text-right text-zinc-300">{formatPrice(t.price)}</span>
                <span className="text-right text-zinc-500">{t.qty}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </section>
  );
}
