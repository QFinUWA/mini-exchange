"use client";

import { useState } from "react";
import { useOpenOrders, useCancelOrder, useCancelAll, formatPrice } from "@/lib/exchange";

type SideFilter = "all" | "buy" | "sell";

export function OpenOrders() {
  const orders = useOpenOrders();
  const cancelOrder = useCancelOrder();
  const cancelAll = useCancelAll();

  const [busyId, setBusyId] = useState<string | null>(null);
  const [cancellingAll, setCancellingAll] = useState(false);
  const [symbolFilter, setSymbolFilter] = useState<string>("all");
  const [sideFilter, setSideFilter] = useState<SideFilter>("all");

  const open = orders.filter(
    (o) => o.status === "open" || o.status === "partial"
  );

  const symbols = [...new Set(open.map((o) => o.symbol))].sort();

  const filtered = open.filter((o) => {
    if (symbolFilter !== "all" && o.symbol !== symbolFilter) return false;
    if (sideFilter !== "all" && o.side !== sideFilter) return false;
    return true;
  });

  async function handleCancel(orderId: string) {
    setBusyId(orderId);
    try {
      await cancelOrder(orderId);
    } catch {
    } finally {
      setBusyId(null);
    }
  }

  const cancelLabel = [
    "Cancel",
    symbolFilter !== "all" ? symbolFilter : "",
    sideFilter === "buy" ? "Bid" : sideFilter === "sell" ? "Ask" : "",
  ].filter(Boolean).join(" ");

  async function handleCancelAll() {
    setCancellingAll(true);
    try {
      await cancelAll(
        symbolFilter !== "all" ? symbolFilter : undefined,
        sideFilter !== "all" ? sideFilter : undefined,
      );
    } catch {
    } finally {
      setCancellingAll(false);
    }
  }

  const pill = (label: string, active: boolean, onClick: () => void, variant?: "buy" | "sell") => {
    let cls = "px-2 py-0.5 text-[10px] font-medium rounded-md cursor-pointer transition-all ";
    if (active) {
      if (variant === "buy") cls += "bg-emerald-500/15 text-emerald-400";
      else if (variant === "sell") cls += "bg-red-500/15 text-red-400";
      else cls += "bg-zinc-700 text-white";
    } else {
      cls += "text-zinc-600 hover:text-zinc-300 hover:bg-zinc-800/50";
    }
    return <button onClick={onClick} className={cls}>{label}</button>;
  };

  return (
    <section className="flex-1 min-h-0 flex flex-col border-b border-zinc-800/60">
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-zinc-800/60 shrink-0">
        <div className="flex items-center gap-2">
          <h2 className="text-xs font-semibold text-zinc-400">Open Orders</h2>
          <span className="text-xs text-zinc-600">{filtered.length}</span>
        </div>
        <button
          onClick={handleCancelAll}
          disabled={open.length === 0 || cancellingAll}
          className="px-2 py-1 text-[10px] font-medium text-red-400/80 rounded-md hover:bg-red-500/10 disabled:opacity-30 disabled:cursor-not-allowed transition-all"
        >
          {cancellingAll ? "..." : symbolFilter === "all" && sideFilter === "all" ? "Cancel All" : cancelLabel}
        </button>
      </div>

      {symbols.length > 0 && (
        <div className="flex items-center gap-1 px-3 py-1.5 border-b border-zinc-800/40 flex-wrap shrink-0">
          {pill("All", symbolFilter === "all", () => setSymbolFilter("all"))}
          {symbols.map((s) => pill(s, symbolFilter === s, () => setSymbolFilter(symbolFilter === s ? "all" : s)))}
          <span className="mx-1 w-px h-3 bg-zinc-800" />
          {pill("Bid", sideFilter === "buy", () => setSideFilter(sideFilter === "buy" ? "all" : "buy"), "buy")}
          {pill("Ask", sideFilter === "sell", () => setSideFilter(sideFilter === "sell" ? "all" : "sell"), "sell")}
        </div>
      )}

      <div className="text-[11px] tabular-nums flex flex-col flex-1 min-h-0">
        <div className="grid grid-cols-[1fr_40px_60px_50px_50px_50px] px-3 py-1.5 text-[10px] text-zinc-600 border-b border-zinc-800/40 shrink-0">
          <span>Symbol</span>
          <span>Side</span>
          <span className="text-right">Price</span>
          <span className="text-right">Qty</span>
          <span className="text-right">Fill</span>
          <span className="text-right"></span>
        </div>

        <div className="overflow-y-auto flex-1 min-h-0">
        {filtered.length === 0 ? (
          <div className="px-3 py-3 text-zinc-600 text-xs">
            {open.length === 0 ? "No open orders" : "No matching orders"}
          </div>
        ) : (
          filtered.map((o) => (
            <div
              key={o._id}
              className="grid grid-cols-[1fr_40px_60px_50px_50px_50px] px-3 py-1 border-b border-zinc-800/30 hover:bg-zinc-800/20 items-center"
            >
              <span className="text-zinc-300 truncate font-medium">
                {o.symbol}
              </span>
              <span
                className={`font-medium ${
                  o.side === "buy" ? "text-emerald-400" : "text-red-400"
                }`}
              >
                {o.side === "buy" ? "BID" : "ASK"}
              </span>
              <span className="text-right text-zinc-300">{formatPrice(o.price)}</span>
              <span className="text-right text-zinc-400">{o.qty}</span>
              <span className="text-right text-zinc-600">{o.filledQty}</span>
              <span className="text-right">
                <button
                  onClick={() => handleCancel(o._id)}
                  disabled={busyId === o._id}
                  className="px-1.5 py-0.5 text-[10px] text-zinc-500 rounded hover:bg-zinc-700 hover:text-white disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                >
                  {busyId === o._id ? "..." : "x"}
                </button>
              </span>
            </div>
          ))
        )}
        </div>
      </div>
    </section>
  );
}
