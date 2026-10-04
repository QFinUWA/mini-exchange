"use client";

import { useMemo, useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import { useLeaderboard, usePnlHistory } from "@/lib/exchange";
import Header from "@/components/Header";

const COLORS = [
  "#3b82f6", "#ef4444", "#22c55e", "#f59e0b", "#8b5cf6",
  "#ec4899", "#06b6d4", "#f97316", "#14b8a6", "#a855f7",
  "#6366f1", "#10b981", "#e11d48", "#0ea5e9", "#84cc16",
  "#d946ef", "#f43f5e", "#2dd4bf", "#fbbf24", "#818cf8",
];

function stableColor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) {
    h = (h * 31 + name.charCodeAt(i)) | 0;
  }
  return COLORS[((h % COLORS.length) + COLORS.length) % COLORS.length];
}

type PnlPoint = { snapshotAt: number; totalPnl: number; realizedPnl: number };
type PnlHistory = Record<string, PnlPoint[]>;
type PnlMode = "total" | "realized";

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

function formatTime(ts: number): string {
  const d = new Date(ts);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm}`;
}

function buildChartData(history: PnlHistory, mode: PnlMode): {
  data: Array<Record<string, number>>;
  teams: string[];
} {
  const allTeams = Object.keys(history);
  const tsSet = new Set<number>();
  for (const team of allTeams) {
    for (const point of history[team]) tsSet.add(point.snapshotAt);
  }
  const timestamps = Array.from(tsSet).sort((a, b) => a - b);

  const lastByTeam: Record<string, number | undefined> = {};
  const data = timestamps.map((ts) => {
    const row: Record<string, number> = { snapshotAt: ts };
    for (const team of allTeams) {
      const point = history[team].find((p) => p.snapshotAt === ts);
      if (point) {
        lastByTeam[team] = mode === "realized" ? point.realizedPnl : point.totalPnl;
      }
      const v = lastByTeam[team];
      if (v !== undefined) row[team] = v;
    }
    return row;
  });

  const teams = allTeams.sort((a, b) => (lastByTeam[b] ?? 0) - (lastByTeam[a] ?? 0));

  return { data, teams };
}

function rankBadge(rank: number) {
  if (rank === 1) return <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-500/20 text-amber-400 text-xs font-bold">1</span>;
  if (rank === 2) return <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-zinc-400/20 text-zinc-300 text-xs font-bold">2</span>;
  if (rank === 3) return <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-orange-500/20 text-orange-400 text-xs font-bold">3</span>;
  return <span className="text-zinc-600 text-sm tabular-nums pl-1">{rank}</span>;
}

export default function LeaderboardPage() {
  const leaderboard = useLeaderboard();
  const history = usePnlHistory() as PnlHistory | null;

  const [pnlMode, setPnlMode] = useState<PnlMode>("total");

  const chart = useMemo(
    () => (history ? buildChartData(history, pnlMode) : { data: [], teams: [] }),
    [history, pnlMode],
  );

  const filteredLeaderboard = leaderboard.filter((r) => r.username && r.username.trim().length > 0);
  const scored = filteredLeaderboard.some((r) => (r.days ?? 0) > 0);

  return (
    <div className="h-screen flex flex-col overflow-hidden">
      <Header />

      <main className="flex-1 flex gap-4 p-4 min-h-0">
        <div className="flex-[2] min-w-0 bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-5 flex flex-col">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-lg font-semibold">P&L Chart</h2>
            <div className="flex bg-zinc-800 rounded-lg p-0.5 text-xs">
              <button
                onClick={() => setPnlMode("total")}
                className={`px-3 py-1 rounded-md transition-colors ${
                  pnlMode === "total"
                    ? "bg-zinc-700 text-zinc-200"
                    : "text-zinc-500 hover:text-zinc-400"
                }`}
              >
                Total
              </button>
              <button
                onClick={() => setPnlMode("realized")}
                className={`px-3 py-1 rounded-md transition-colors ${
                  pnlMode === "realized"
                    ? "bg-zinc-700 text-zinc-200"
                    : "text-zinc-500 hover:text-zinc-400"
                }`}
              >
                Realized
              </button>
            </div>
          </div>
          <div className="flex-1 min-h-0">
            {history === null ? (
              <div className="h-full flex items-center justify-center text-zinc-500 text-sm">
                Loading chart...
              </div>
            ) : chart.teams.length === 0 || chart.data.length === 0 ? (
              <div className="h-full flex items-center justify-center text-zinc-500 text-sm">
                No P&L history yet
              </div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={chart.data} margin={{ top: 8, right: 16, left: 8, bottom: 8 }}>
                  <CartesianGrid stroke="#27272a" strokeDasharray="3 3" />
                  <XAxis
                    dataKey="snapshotAt"
                    type="number"
                    domain={["dataMin", "dataMax"]}
                    scale="time"
                    tickFormatter={formatTime}
                    stroke="#52525b"
                    tick={{ fill: "#71717a", fontSize: 12 }}
                  />
                  <YAxis
                    stroke="#52525b"
                    tick={{ fill: "#71717a", fontSize: 12 }}
                    tickFormatter={(v: number) =>
                      v.toLocaleString(undefined, { maximumFractionDigits: 0 })
                    }
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "#18181b",
                      border: "1px solid #27272a",
                      borderRadius: 12,
                      color: "#e4e4e7",
                      fontSize: 12,
                      padding: "8px 12px",
                    }}
                    labelStyle={{ color: "#71717a" }}
                    labelFormatter={(label) => formatTime(Number(label))}
                    formatter={(value) => formatPnl(Number(value))}
                    itemSorter={(item) => -(Number(item.value) || 0)}
                  />
                  <Legend wrapperStyle={{ color: "#a1a1aa", fontSize: 12 }} />
                  {chart.teams.map((team) => (
                    <Line
                      key={team}
                      type="monotone"
                      dataKey={team}
                      stroke={stableColor(team)}
                      strokeWidth={2}
                      dot={false}
                      isAnimationActive={false}
                      connectNulls
                    />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            )}
          </div>
        </div>

        <div className="w-[340px] shrink-0 bg-zinc-900/50 rounded-2xl border border-zinc-800/60 flex flex-col overflow-hidden">
          <div className="px-5 py-4 border-b border-zinc-800/60 shrink-0">
            <h2 className="text-lg font-semibold">Rankings</h2>
            <p className="mt-1 text-xs text-zinc-500">
              {scored
                ? "Score = mean daily P&L - std of daily P&L (completed days only)"
                : "Ranked by today's P&L until the first day ends"}
            </p>
          </div>
          <div className="overflow-y-auto flex-1 min-h-0">
            {filteredLeaderboard.length === 0 ? (
              <div className="p-8 text-zinc-500 text-sm text-center">No teams yet</div>
            ) : (
              <div className="divide-y divide-zinc-800/30">
                {filteredLeaderboard.map((row, idx) => {
                  const rank = idx + 1;
                  return (
                    <div
                      key={row.userId}
                      className={`flex items-center gap-3 px-5 py-3 transition-colors hover:bg-zinc-800/20 ${
                        rank === 1 ? "bg-amber-500/5" : rank === 2 ? "bg-zinc-400/5" : rank === 3 ? "bg-orange-500/5" : ""
                      }`}
                    >
                      <div className="w-8 flex justify-center shrink-0">
                        {rankBadge(rank)}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className={`font-medium truncate ${rank <= 3 ? "text-white" : "text-zinc-300"}`}>
                          {row.username}
                        </div>
                        <div className="text-[11px] text-zinc-500 tabular-nums truncate">
                          {row.days > 0
                            ? `${row.days}d · mean ${formatPnl(row.meanDailyPnl)} · sd ${row.stdDailyPnl.toFixed(2)}`
                            : "no completed days"}
                          {row.busts > 0 && <span className="text-red-400/80"> · {row.busts} bust{row.busts === 1 ? "" : "s"}</span>}
                        </div>
                      </div>
                      <div className="text-right shrink-0 tabular-nums">
                        {scored ? (
                          <>
                            <div className={`font-bold ${pnlClass(row.score)}`}>{formatPnl(row.score)}</div>
                            <div className={`text-[11px] ${pnlClass(row.totalPnl)}`}>today {formatPnl(row.totalPnl)}</div>
                          </>
                        ) : (
                          <div className={`font-bold ${pnlClass(row.totalPnl)}`}>{formatPnl(row.totalPnl)}</div>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
