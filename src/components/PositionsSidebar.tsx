"use client";

import { usePositions, useOpenOrders, formatPrice } from "@/lib/exchange";

function pnlClass(v: number) {
  if (v > 0) return "text-emerald-400";
  if (v < 0) return "text-red-400";
  return "text-zinc-500";
}

function fmt(v: number, digits = 2) {
  if (!Number.isFinite(v)) return "-";
  const s = v.toFixed(digits);
  return v > 0 ? `+${s}` : s;
}

export function PositionsSidebar() {
  const positions = usePositions();
  const allOrders = useOpenOrders();

  const restingBySymbol = new Map<string, { bid: number; ask: number }>();
  for (const o of allOrders) {
    if (o.status === "open" || o.status === "partial") {
      const rest = o.qty - o.filledQty;
      const entry = restingBySymbol.get(o.symbol) ?? { bid: 0, ask: 0 };
      if (o.side === "buy") entry.bid += rest;
      else entry.ask += rest;
      restingBySymbol.set(o.symbol, entry);
    }
  }

  const rows = positions;
  const totalUnrealized = rows.reduce((s, p) => s + (p.unrealizedPnl ?? 0), 0);
  const totalRealized = rows.reduce((s, p) => s + (p.realizedPnl ?? 0), 0);
  const total = totalUnrealized + totalRealized;

  return (
    <section className="flex-1 min-h-0 flex flex-col border-b border-zinc-800/60">
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-zinc-800/60 shrink-0">
        <h2 className="text-xs font-semibold text-zinc-400">Positions</h2>
        <span className="text-xs text-zinc-600">{rows.length}</span>
      </div>

      <div className="flex flex-col flex-1 min-h-0 text-[11px] tabular-nums">
        {/* Column definitions shared across header/body/footer via identical table-layout:fixed + colgroup */}
        {/* widths: auto | 36px | 48px | 40px | 68px | 76px */}
        <table className="w-full table-fixed shrink-0">
          <colgroup>
            <col />
            <col className="w-9" />
            <col className="w-12" />
            <col className="w-10" />
            <col className="w-[68px]" />
            <col className="w-[76px]" />
          </colgroup>
          <thead>
            <tr className="text-[10px] text-zinc-600 border-b border-zinc-800/40">
              <th className="text-left font-normal px-3 py-1.5">Symbol</th>
              <th className="text-right font-normal px-1 py-1.5">Qty</th>
              <th className="text-right font-normal px-1 py-1.5">Resting</th>
              <th className="text-right font-normal px-1 py-1.5">Avg</th>
              <th className="text-right font-normal px-1 py-1.5">Pos P&L</th>
              <th className="text-right font-normal px-2 py-1.5">Realized</th>
            </tr>
          </thead>
        </table>

        <div className="overflow-y-auto flex-1 min-h-0">
          {rows.length === 0 ? (
            <div className="px-3 py-3 text-zinc-600 text-xs">No positions</div>
          ) : (
            <table className="w-full table-fixed">
              <colgroup>
                <col />
                <col className="w-9" />
                <col className="w-12" />
                <col className="w-10" />
                <col className="w-[68px]" />
                <col className="w-[76px]" />
              </colgroup>
              <tbody>
                {rows.map((p) => (
                  <tr
                    key={p.symbol}
                    className="border-b border-zinc-800/30 hover:bg-zinc-800/20"
                  >
                    <td className="text-left text-zinc-200 font-medium px-3 py-1 truncate">{p.symbol}</td>
                    <td
                      className={`text-right font-medium px-1 py-1 ${
                        p.qty > 0 ? "text-emerald-400" : p.qty < 0 ? "text-red-400" : "text-zinc-500"
                      }`}
                    >
                      {p.qty}
                    </td>
                    <td className="text-right px-1 py-1">
                      {(() => {
                        const r = restingBySymbol.get(p.symbol);
                        if (!r || (r.bid === 0 && r.ask === 0)) return <span className="text-zinc-700">-</span>;
                        return (
                          <>
                            <span className="text-emerald-400/60">{r.bid}</span>
                            <span className="text-zinc-600">/</span>
                            <span className="text-red-400/60">{r.ask}</span>
                          </>
                        );
                      })()}
                    </td>
                    <td className="text-right text-zinc-400 px-1 py-1">
                      {p.qty === 0 ? "-" : formatPrice(p.avgEntryPrice)}
                    </td>
                    <td className={`text-right px-1 py-1 ${pnlClass(p.unrealizedPnl ?? 0)}`}>
                      {p.unrealizedPnl !== undefined ? fmt(p.unrealizedPnl) : "-"}
                    </td>
                    <td className={`text-right px-2 py-1 ${pnlClass(p.realizedPnl)}`}>
                      {fmt(p.realizedPnl)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <table className="w-full table-fixed shrink-0">
          <colgroup>
            <col />
            <col className="w-9" />
            <col className="w-12" />
            <col className="w-10" />
            <col className="w-[68px]" />
            <col className="w-[76px]" />
          </colgroup>
          <tfoot>
            <tr className="bg-zinc-900/50 border-t border-zinc-800/40">
              <td className="text-left text-xs text-zinc-500 px-3 py-1.5">Total</td>
              <td></td>
              <td></td>
              <td></td>
              <td className={`text-right px-1 py-1.5 ${pnlClass(totalUnrealized)}`}>
                {fmt(totalUnrealized)}
              </td>
              <td className={`text-right px-2 py-1.5 ${pnlClass(totalRealized)}`}>{fmt(totalRealized)}</td>
            </tr>
            <tr className="bg-zinc-900/50">
              <td className="text-left text-xs text-zinc-500 px-3 py-1.5">Total P&amp;L</td>
              <td></td>
              <td></td>
              <td></td>
              <td></td>
              <td className={`text-right text-sm font-bold px-2 py-1.5 ${pnlClass(total)}`}>
                {fmt(total)}
              </td>
            </tr>
          </tfoot>
        </table>
      </div>
    </section>
  );
}
