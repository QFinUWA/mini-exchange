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
import { usePnlHistory } from "@/lib/exchange";
import Header from "@/components/Header";

const COLORS = [
  "#3b82f6", "#ef4444", "#22c55e", "#f59e0b", "#8b5cf6",
  "#ec4899", "#06b6d4", "#f97316", "#14b8a6", "#a855f7",
  "#6366f1", "#10b981", "#e11d48", "#0ea5e9", "#84cc16",
  "#d946ef", "#f43f5e", "#2dd4bf", "#fbbf24", "#818cf8",
];

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

export default function ChartPage() {
  const history = usePnlHistory() as PnlHistory | null;

  const [pnlMode, setPnlMode] = useState<PnlMode>("total");

  const chart = useMemo(
    () => (history ? buildChartData(history, pnlMode) : { data: [], teams: [] }),
    [history, pnlMode],
  );

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 px-6 py-6 max-w-6xl w-full mx-auto">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-2xl font-bold">P&L Chart</h1>
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

        {history === null ? (
          <div className="bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-4 h-[calc(100vh-180px)] flex items-center justify-center text-zinc-500 text-sm">
            Loading chart...
          </div>
        ) : chart.teams.length === 0 || chart.data.length === 0 ? (
          <div className="bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-4 h-[calc(100vh-180px)] flex items-center justify-center text-zinc-500 text-sm">
            No P&amp;L history yet
          </div>
        ) : (
          <div className="bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-5 h-[calc(100vh-180px)]">
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
                {chart.teams.map((team, idx) => (
                  <Line
                    key={team}
                    type="monotone"
                    dataKey={team}
                    stroke={COLORS[idx % COLORS.length]}
                    strokeWidth={2}
                    dot={false}
                    isAnimationActive={false}
                    connectNulls
                  />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </main>
    </div>
  );
}
