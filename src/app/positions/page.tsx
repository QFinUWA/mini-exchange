"use client";

import { useMemo } from "react";
import { useAllPositions, useVolumeMatrix } from "@/lib/exchange";
import Header from "@/components/Header";

function formatPnl(value: number): string {
  const sign = value > 0 ? "+" : "";
  return `${sign}${value.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function pnlClass(value: number): string {
  if (value > 0) return "text-emerald-400";
  if (value < 0) return "text-red-400";
  return "text-zinc-500";
}

function qtyClass(value: number): string {
  if (value > 0) return "text-emerald-400";
  if (value < 0) return "text-red-400";
  return "text-zinc-500";
}

export default function PositionsPage() {
  const positions = useAllPositions();

  const symbols = useMemo(() => {
    if (!positions.length) return [];
    return [...new Set(positions.map((p) => p.symbol))].sort();
  }, [positions]);

  const teamData = useMemo(() => {
    if (!positions.length) return [];
    const byTeam = new Map<
      string,
      {
        positions: Map<string, typeof positions[0]>;
        totalPnl: number;
        realizedPnl: number;
        absPosition: number;
      }
    >();

    for (const p of positions) {
      if (!byTeam.has(p.username)) {
        byTeam.set(p.username, { positions: new Map(), totalPnl: 0, realizedPnl: 0, absPosition: 0 });
      }
      const team = byTeam.get(p.username)!;
      team.positions.set(p.symbol, p);
      team.totalPnl += p.realizedPnl + p.unrealizedPnl;
      team.realizedPnl += p.realizedPnl;
      team.absPosition += Math.abs(p.qty);
    }

    return [...byTeam.entries()]
      .map(([username, data]) => ({ username, ...data }))
      .sort((a, b) => b.totalPnl - a.totalPnl);
  }, [positions]);

  const colTotals = useMemo(() => {
    const totals = new Map<string, number>();
    if (!positions.length) return totals;
    for (const p of positions) {
      totals.set(p.symbol, (totals.get(p.symbol) ?? 0) + p.qty);
    }
    return totals;
  }, [positions]);

  const colCount = symbols.length + 4;

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 px-6 py-6 max-w-6xl w-full mx-auto">
        <h1 className="text-2xl font-bold mb-6">All Positions</h1>

        <div className="bg-zinc-900/50 rounded-2xl border border-zinc-800/60 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-zinc-500 border-b border-zinc-800/60">
                <th className="px-5 py-3.5">Team</th>
                {symbols.map((s) => (
                  <th key={s} className="px-4 py-3.5 text-right">{s}</th>
                ))}
                <th className="px-5 py-3.5 text-right">Position</th>
                <th className="px-5 py-3.5 text-right">Realized</th>
                <th className="px-5 py-3.5 text-right">Total P&L</th>
              </tr>
            </thead>
            <tbody>
              {teamData.length === 0 ? (
                <tr>
                  <td colSpan={colCount} className="px-5 py-8 text-center text-zinc-500">No positions</td>
                </tr>
              ) : (
                <>
                  {teamData.map((team) => (
                    <tr
                      key={team.username}
                      className="border-b border-zinc-800/30 hover:bg-zinc-800/20 transition-colors"
                    >
                      <td className="px-5 py-3 font-medium text-zinc-200">{team.username}</td>
                      {symbols.map((s) => {
                        const pos = team.positions.get(s);
                        const qty = pos?.qty ?? 0;
                        return (
                          <td
                            key={s}
                            className={`px-4 py-3 text-right tabular-nums font-medium ${qtyClass(qty)}`}
                            title={qty !== 0 && pos ? `avg ${pos.avgEntryPrice.toFixed(2)} | pnl ${formatPnl(pos.realizedPnl + pos.unrealizedPnl)}` : undefined}
                          >
                            {qty === 0 ? <span className="text-zinc-700">-</span> : qty}
                          </td>
                        );
                      })}
                      <td className="px-5 py-3 text-right tabular-nums text-zinc-400">
                        {team.absPosition === 0 ? <span className="text-zinc-700">-</span> : team.absPosition}
                      </td>
                      <td className={`px-5 py-3 text-right tabular-nums ${pnlClass(team.realizedPnl)}`}>
                        {formatPnl(team.realizedPnl)}
                      </td>
                      <td className={`px-5 py-3 text-right tabular-nums font-bold ${pnlClass(team.totalPnl)}`}>
                        {formatPnl(team.totalPnl)}
                      </td>
                    </tr>
                  ))}
                  <tr className="bg-zinc-900/80 border-t border-zinc-800/60">
                    <td className="px-5 py-3 font-medium text-zinc-400">Net</td>
                    {symbols.map((s) => {
                      const net = colTotals.get(s) ?? 0;
                      return (
                        <td key={s} className={`px-4 py-3 text-right tabular-nums font-bold ${qtyClass(net)}`}>
                          {net === 0 ? <span className="text-zinc-600">0</span> : net}
                        </td>
                      );
                    })}
                    <td className="px-5 py-3 text-right tabular-nums text-zinc-400">
                      {teamData.reduce((s, t) => s + t.absPosition, 0)}
                    </td>
                    <td className={`px-5 py-3 text-right tabular-nums ${pnlClass(
                      teamData.reduce((s, t) => s + t.realizedPnl, 0)
                    )}`}>
                      {formatPnl(teamData.reduce((s, t) => s + t.realizedPnl, 0))}
                    </td>
                    <td className={`px-5 py-3 text-right tabular-nums font-bold ${pnlClass(
                      teamData.reduce((s, t) => s + t.totalPnl, 0)
                    )}`}>
                      {formatPnl(teamData.reduce((s, t) => s + t.totalPnl, 0))}
                    </td>
                  </tr>
                </>
              )}
            </tbody>
          </table>
        </div>

        <VolumeMatrix />
      </main>
    </div>
  );
}

function formatVol(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toFixed(0);
}

function VolumeMatrix() {
  const matrix = useVolumeMatrix();

  const { users, grid, maxVol } = useMemo(() => {
    if (!matrix || matrix.length === 0) return { users: [], grid: new Map<string, number>(), maxVol: 0 };

    const userSet = new Set<string>();
    const grid = new Map<string, number>();
    let maxVol = 0;

    for (const e of matrix) {
      userSet.add(e.buyer);
      userSet.add(e.seller);
      const key = `${e.buyer}|${e.seller}`;
      grid.set(key, (grid.get(key) ?? 0) + e.volume);
    }

    for (const v of grid.values()) {
      if (v > maxVol) maxVol = v;
    }

    return { users: [...userSet].sort(), grid, maxVol };
  }, [matrix]);

  if (!matrix || users.length === 0) {
    return (
      <div className="mt-6 bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-6">
        <h2 className="text-lg font-semibold text-zinc-100 mb-4">Volume Matrix</h2>
        <p className="text-zinc-500 text-sm text-center py-4">No trades yet</p>
      </div>
    );
  }

  return (
    <div className="mt-6 bg-zinc-900/50 rounded-2xl border border-zinc-800/60 overflow-hidden">
      <div className="p-5 pb-0">
        <h2 className="text-lg font-semibold text-zinc-100">Volume Matrix</h2>
        <p className="text-xs text-zinc-500 mt-1">$ volume traded between users (buyer on left, seller on top)</p>
      </div>
      <div className="overflow-x-auto p-5">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-zinc-500 border-b border-zinc-800/60">
              <th className="px-3 py-2.5">Buyer \ Seller</th>
              {users.map((u) => (
                <th key={u} className="px-3 py-2.5 text-right">{u}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {users.map((buyer) => (
              <tr key={buyer} className="border-b border-zinc-800/30">
                <td className="px-3 py-2.5 font-medium text-zinc-200">{buyer}</td>
                {users.map((seller) => {
                  if (buyer === seller) {
                    return (
                      <td key={seller} className="px-3 py-2.5 text-right">
                        <span className="text-zinc-800">-</span>
                      </td>
                    );
                  }
                  const vol = grid.get(`${buyer}|${seller}`) ?? 0;
                  const intensity = maxVol > 0 ? vol / maxVol : 0;
                  return (
                    <td
                      key={seller}
                      className="px-3 py-2.5 text-right tabular-nums"
                      style={vol > 0 ? { backgroundColor: `rgba(59, 130, 246, ${0.08 + intensity * 0.35})` } : undefined}
                    >
                      {vol === 0 ? (
                        <span className="text-zinc-700">0</span>
                      ) : (
                        <span className="text-zinc-200" title={`$${vol.toFixed(2)}`}>
                          {formatVol(vol)}
                        </span>
                      )}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
