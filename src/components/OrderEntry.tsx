"use client";

import { useEffect, useState } from "react";
import { usePlaceOrder, type Product } from "@/lib/exchange";

interface Props {
  products: Product[];
}

type OrderResult =
  | { kind: "ok"; status: string; fillCount: number }
  | { kind: "err"; message: string };

export function OrderEntry({ products }: Props) {
  const placeOrder = usePlaceOrder();

  const [symbol, setSymbol] = useState<string>("");
  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [price, setPrice] = useState("");
  const [qty, setQty] = useState("");
  const [orderType, setOrderType] = useState<"limit" | "ioc">("limit");
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<OrderResult | null>(null);

  useEffect(() => {
    if (!symbol && products.length > 0) {
      setSymbol(products[0].symbol);
    }
  }, [products, symbol]);

  useEffect(() => {
    if (!result) return;
    const t = setTimeout(() => setResult(null), 4000);
    return () => clearTimeout(t);
  }, [result]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!symbol) return;
    const priceNum = Number(price);
    const qtyNum = Number(qty);
    if (!Number.isFinite(priceNum) || priceNum <= 0) {
      setResult({ kind: "err", message: "Invalid price" });
      return;
    }
    if (!Number.isFinite(qtyNum) || qtyNum <= 0) {
      setResult({ kind: "err", message: "Invalid qty" });
      return;
    }
    setSubmitting(true);
    try {
      const res = await placeOrder(symbol, side, priceNum, qtyNum, orderType);
      setResult({
        kind: "ok",
        status: res.status,
        fillCount: Array.isArray(res.fills) ? res.fills.length : 0,
      });
      setQty("");
    } catch (err) {
      setResult({
        kind: "err",
        message: err instanceof Error ? err.message : "Order failed",
      });
    } finally {
      setSubmitting(false);
    }
  }

  const sideBtn = (target: "buy" | "sell", label: string) => {
    const active = side === target;
    const activeClass =
      target === "buy"
        ? "bg-emerald-500/15 text-emerald-400 border-emerald-500/30"
        : "bg-red-500/15 text-red-400 border-red-500/30";
    return (
      <button
        type="button"
        onClick={() => setSide(target)}
        className={`flex-1 py-2 text-xs font-bold uppercase rounded-lg border transition-all ${
          active
            ? activeClass
            : "bg-zinc-900 text-zinc-500 border-zinc-800 hover:text-zinc-300"
        }`}
      >
        {label}
      </button>
    );
  };

  const typeBtn = (target: "limit" | "ioc", label: string) => {
    const active = orderType === target;
    return (
      <button
        type="button"
        onClick={() => setOrderType(target)}
        className={`flex-1 py-1.5 text-[10px] font-medium uppercase rounded-md border transition-all ${
          active
            ? "bg-zinc-800 text-white border-zinc-700"
            : "bg-zinc-900 text-zinc-500 border-zinc-800 hover:text-zinc-300"
        }`}
      >
        {label}
      </button>
    );
  };

  return (
    <form onSubmit={onSubmit} className="p-3 flex flex-col gap-2.5">
      <div className="text-xs font-semibold text-zinc-400">Order Entry</div>

      <select
        value={symbol}
        onChange={(e) => setSymbol(e.target.value)}
        className="bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500/40"
      >
        {products.length === 0 && <option value="">No products</option>}
        {products.map((p) => (
          <option key={p.id} value={p.symbol}>
            {p.symbol} - {p.name}
          </option>
        ))}
      </select>

      <div className="flex gap-1.5">
        {sideBtn("buy", "Bid")}
        {sideBtn("sell", "Ask")}
      </div>

      <div className="flex gap-1.5">
        {typeBtn("limit", "Limit")}
        {typeBtn("ioc", "IOC")}
      </div>

      <label className="flex flex-col gap-1">
        <span className="text-[10px] font-medium text-zinc-500">Price</span>
        <input
          type="number"
          step="0.01"
          min="0"
          value={price}
          onChange={(e) => setPrice(e.target.value)}
          placeholder="0.00"
          className="bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-sm tabular-nums text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500/40"
        />
      </label>

      <label className="flex flex-col gap-1">
        <span className="text-[10px] font-medium text-zinc-500">Quantity</span>
        <input
          type="number"
          step="1"
          min="0"
          value={qty}
          onChange={(e) => setQty(e.target.value)}
          placeholder="0"
          className="bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-sm tabular-nums text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500/40"
        />
      </label>

      <button
        type="submit"
        disabled={submitting || !symbol}
        className={`py-2.5 text-sm font-bold uppercase rounded-lg transition-all ${
          side === "buy"
            ? "bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-500/20"
            : "bg-red-600 hover:bg-red-500 text-white shadow-lg shadow-red-500/20"
        } disabled:opacity-50 disabled:cursor-not-allowed disabled:shadow-none`}
      >
        {submitting ? "..." : `${side === "buy" ? "bid" : "ask"} ${orderType}`}
      </button>

      {result && (
        <div
          className={`text-xs px-3 py-2 rounded-lg ${
            result.kind === "ok"
              ? "text-zinc-300 bg-zinc-800/50"
              : "text-red-300 bg-red-500/10"
          }`}
        >
          {result.kind === "ok"
            ? `${result.status}${result.fillCount > 0 ? ` - ${result.fillCount} fill${result.fillCount === 1 ? "" : "s"}` : ""}`
            : result.message}
        </div>
      )}
    </form>
  );
}
