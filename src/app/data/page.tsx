"use client";

import { useMemo, useState } from "react";
import Header from "@/components/Header";
import {
  apiUrl,
  formatMoney,
  formatPrice,
  useDailyResults,
  useDataFiles,
  usePolled,
  type OptionWindow,
} from "@/lib/exchange";

function pnlClass(v: number) {
  if (v > 0) return "text-emerald-400";
  if (v < 0) return "text-red-400";
  return "text-zinc-500";
}

function utcTime(sec: number) {
  return new Date(sec * 1000).toISOString().slice(11, 16);
}

function formatBytes(n: number) {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

const card = "rounded-2xl border border-zinc-800/60 bg-zinc-900/50";

export default function DataPage() {
  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="mx-auto w-full max-w-6xl px-6 py-8 space-y-6">
        <h1 className="text-2xl font-bold text-zinc-100">Data</h1>
        <div className="grid gap-6 lg:grid-cols-2">
          <OptionResults />
          <DailyResults />
        </div>
        <DataFiles />
      </main>
    </div>
  );
}

function OptionResults() {
  const [expiry, setExpiry] = useState(5);
  const windows = usePolled<OptionWindow[]>(`/api/option-windows?expiry=${expiry}&limit=50`, 5000);
  const upRate = windows && windows.length ? windows.filter((w) => w.upWon).length / windows.length : null;

  return (
    <section className={`${card} flex flex-col max-h-[520px]`}>
      <div className="flex items-center gap-3 px-5 py-4 border-b border-zinc-800/60">
        <h2 className="text-lg font-semibold">Option settlements</h2>
        <div className="ml-auto flex bg-zinc-800 rounded-lg p-0.5 text-xs">
          {[5, 15, 60].map((e) => (
            <button
              key={e}
              onClick={() => setExpiry(e)}
              className={`px-3 py-1 rounded-md ${expiry === e ? "bg-zinc-700 text-zinc-200" : "text-zinc-500 hover:text-zinc-400"}`}
            >
              {e}m
            </button>
          ))}
        </div>
      </div>
      {upRate !== null && (
        <div className="px-5 pt-3 text-xs text-zinc-500">
          Up won {(upRate * 100).toFixed(0)}% of the last {windows!.length} windows
        </div>
      )}
      <div className="overflow-y-auto px-5 py-3">
        {!windows ? (
          <div className="text-sm text-zinc-500">Loading...</div>
        ) : windows.length === 0 ? (
          <div className="text-sm text-zinc-500">No settled {expiry}m windows yet</div>
        ) : (
          <table className="w-full text-sm tabular-nums">
            <thead>
              <tr className="text-xs text-zinc-500 text-left">
                <th className="font-normal py-1">Window (UTC)</th>
                <th className="font-normal py-1 text-right">Strike</th>
                <th className="font-normal py-1 text-right">Settle</th>
                <th className="font-normal py-1 text-right">Move</th>
                <th className="font-normal py-1 text-right">Result</th>
              </tr>
            </thead>
            <tbody>
              {windows.map((w) => {
                const move = w.settle - w.strike;
                return (
                  <tr key={`${w.expiryMin}-${w.windowOpen}`} className="border-t border-zinc-800/40">
                    <td className="py-1 text-zinc-300">
                      {utcTime(w.windowOpen)}-{utcTime(w.windowOpen + w.expiryMin * 60)}
                    </td>
                    <td className="py-1 text-right text-zinc-400">{formatPrice(w.strike)}</td>
                    <td className="py-1 text-right text-zinc-400">{formatPrice(w.settle)}</td>
                    <td className={`py-1 text-right ${pnlClass(move)}`}>{move >= 0 ? "+" : ""}{move.toFixed(2)}</td>
                    <td className="py-1 text-right">
                      <span
                        className={`px-1.5 py-0.5 rounded text-[11px] font-semibold ${
                          w.upWon ? "bg-emerald-500/15 text-emerald-400" : "bg-red-500/15 text-red-400"
                        }`}
                      >
                        {w.upWon ? "UP" : "DOWN"}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

function DailyResults() {
  const results = useDailyResults();
  const { days, users } = useMemo(() => {
    const daySet = new Set<string>();
    const users = Object.keys(results ?? {}).sort();
    for (const u of users) for (const r of results![u]) daySet.add(r.day);
    return { days: [...daySet].sort().reverse(), users };
  }, [results]);

  const pnlFor = (user: string, day: string) => {
    const rs = results?.[user]?.filter((r) => r.day === day) ?? [];
    if (!rs.length) return null;
    return { pnl: rs.reduce((s, r) => s + r.pnl, 0), busts: rs.reduce((s, r) => s + r.busts, 0) };
  };

  return (
    <section className={`${card} flex flex-col max-h-[520px]`}>
      <div className="px-5 py-4 border-b border-zinc-800/60">
        <h2 className="text-lg font-semibold">Daily results</h2>
        <p className="mt-1 text-xs text-zinc-500">P&L recorded at the end of each trading day</p>
      </div>
      <div className="overflow-auto px-5 py-3">
        {!results ? (
          <div className="text-sm text-zinc-500">Loading...</div>
        ) : days.length === 0 ? (
          <div className="text-sm text-zinc-500">No trading days have ended yet</div>
        ) : (
          <table className="w-full text-sm tabular-nums">
            <thead>
              <tr className="text-xs text-zinc-500 text-left">
                <th className="font-normal py-1 pr-4">Day</th>
                {users.map((u) => (
                  <th key={u} className="font-normal py-1 px-2 text-right whitespace-nowrap">{u}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {days.map((d) => (
                <tr key={d} className="border-t border-zinc-800/40">
                  <td className="py-1 pr-4 text-zinc-300 whitespace-nowrap">{d}</td>
                  {users.map((u) => {
                    const r = pnlFor(u, d);
                    return (
                      <td key={u} className={`py-1 px-2 text-right ${r ? pnlClass(r.pnl) : "text-zinc-700"}`}>
                        {r ? formatMoney(r.pnl, true) : "-"}
                        {r && r.busts > 0 && <span className="ml-1 text-[10px] text-red-400/80">{r.busts}B</span>}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

function DataFiles() {
  const files = useDataFiles();
  const byDate = useMemo(() => {
    const m = new Map<string, NonNullable<typeof files>>();
    for (const f of files ?? []) m.set(f.date, [...(m.get(f.date) ?? []), f]);
    return [...m.entries()].sort((a, b) => b[0].localeCompare(a[0]));
  }, [files]);
  const today = new Date().toISOString().slice(0, 10);

  return (
    <section className={card}>
      <div className="px-5 py-4 border-b border-zinc-800/60">
        <h2 className="text-lg font-semibold">Market data (Parquet)</h2>
        <p className="mt-1 text-xs text-zinc-500">
          Same tables and columns as the handout data, one file per UTC day. Today&apos;s files are rewritten every
          minute. Bots can fetch everything with <code className="text-zinc-300">self.download_data()</code>.
        </p>
      </div>
      <div className="px-5 py-3">
        {!files ? (
          <div className="text-sm text-zinc-500">Loading...</div>
        ) : byDate.length === 0 ? (
          <div className="text-sm text-zinc-500">Nothing recorded yet</div>
        ) : (
          <table className="w-full text-sm">
            <tbody>
              {byDate.map(([date, fs]) => (
                <tr key={date} className="border-t border-zinc-800/40 first:border-t-0">
                  <td className="py-2 pr-6 text-zinc-300 tabular-nums whitespace-nowrap align-top">
                    {date}
                    {date === today && <span className="ml-2 text-[10px] text-amber-400">live</span>}
                  </td>
                  <td className="py-2">
                    <div className="flex flex-wrap gap-2">
                      {fs.sort((a, b) => a.table.localeCompare(b.table)).map((f) => (
                        <a
                          key={f.path}
                          href={apiUrl(`/api/data/${f.path}`)}
                          download={`${f.table}_${f.date}.parquet`}
                          className="px-2.5 py-1 rounded-md bg-zinc-800 border border-zinc-700/60 text-xs text-zinc-300 hover:bg-zinc-700 hover:text-white"
                        >
                          {f.table} <span className="text-zinc-500">{formatBytes(f.bytes)}</span>
                        </a>
                      ))}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}
