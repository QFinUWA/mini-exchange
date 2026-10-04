"use client";

import { useMemo } from "react";
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
import { usePriceHistory } from "@/lib/exchange";
import Header from "@/components/Header";

const COLORS = [
  "#3b82f6", "#ef4444", "#22c55e", "#f59e0b", "#8b5cf6",
  "#ec4899", "#06b6d4", "#f97316", "#14b8a6", "#a855f7",
];

function stableColor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) {
    h = (h * 31 + name.charCodeAt(i)) | 0;
  }
  return COLORS[((h % COLORS.length) + COLORS.length) % COLORS.length];
}

function formatTime(ts: number): string {
  const d = new Date(ts);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm}`;
}

type PriceRow = Record<string, number>;

function buildChartData(history: Record<string, Array<{ mid: number; snapshotAt: number }>>): {
  data: PriceRow[];
  symbols: string[];
} {
  const allSymbols = Object.keys(history).sort();
  const tsSet = new Set<number>();
  for (const sym of allSymbols) {
    for (const point of history[sym]) tsSet.add(point.snapshotAt);
  }
  const timestamps = Array.from(tsSet).sort((a, b) => a - b);

  const lastBySym: Record<string, number | undefined> = {};
  const data = timestamps.map((ts) => {
    const row: PriceRow = { snapshotAt: ts };
    for (const sym of allSymbols) {
      const point = history[sym].find((p) => p.snapshotAt === ts);
      if (point) lastBySym[sym] = point.mid;
      const v = lastBySym[sym];
      if (v !== undefined) row[sym] = v;
    }
    return row;
  });

  return { data, symbols: allSymbols };
}

export default function PricesPage() {
  const history = usePriceHistory();

  const chart = useMemo(
    () => (history ? buildChartData(history) : { data: [], symbols: [] }),
    [history],
  );

  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="flex-1 px-6 py-6 max-w-6xl w-full mx-auto">
        <h1 className="text-2xl font-bold mb-6">Prices</h1>

        {history === null ? (
          <div className="bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-4 h-[calc(100vh-180px)] flex items-center justify-center text-zinc-500 text-sm">
            Loading...
          </div>
        ) : chart.symbols.length === 0 || chart.data.length === 0 ? (
          <div className="bg-zinc-900/50 rounded-2xl border border-zinc-800/60 p-4 h-[calc(100vh-180px)] flex items-center justify-center text-zinc-500 text-sm">
            No price history yet
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
                  domain={["auto", "auto"]}
                  tickFormatter={(v: number) => v.toFixed(2)}
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
                  formatter={(value) => Number(value).toFixed(2)}
                />
                <Legend wrapperStyle={{ color: "#a1a1aa", fontSize: 12 }} />
                {chart.symbols.map((sym) => (
                  <Line
                    key={sym}
                    type="monotone"
                    dataKey={sym}
                    stroke={stableColor(sym)}
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
