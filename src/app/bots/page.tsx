"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Bar, BarChart, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import Header from "@/components/Header";
import { useAuth } from "@/lib/auth";
import { apiFetch, apiUrl, formatMoney, useConfig, usePolled } from "@/lib/exchange";

type BacktestParams = { days: number; dayMin: number; seed?: number };
type BacktestDay = { day: string; pnl: number; busts: number };
type BacktestResult = {
  status: "done" | "error";
  error?: string;
  days: BacktestDay[];
  meanDailyPnl: number;
  stdDailyPnl: number;
  score: number;
  busts: number;
  fills: number;
  volume: number;
  wallSec: number;
  log?: string;
};
type Submission = {
  id: string;
  username: string;
  filename: string;
  createdAt: number;
  params: BacktestParams;
  status: "queued" | "running" | "done" | "error";
  queuePosition?: number;
  startedAt?: number;
  result?: BacktestResult;
};
type BotLeaderboardEntry = {
  username: string;
  submissionId: string;
  filename: string;
  submittedAt: number;
  params: BacktestParams;
  days: number;
  meanDailyPnl: number;
  stdDailyPnl: number;
  score: number;
  busts: number;
};

const card = "rounded-2xl border border-zinc-800/60 bg-zinc-900/50";

function pnlClass(v: number) {
  if (v > 0) return "text-emerald-400";
  if (v < 0) return "text-red-400";
  return "text-zinc-500";
}

function ago(ms: number) {
  const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return new Date(ms).toLocaleDateString();
}

const stripAnsi = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

export default function BotsPage() {
  const [submissions, setSubmissions] = useState<Submission[] | null>(null);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const data = await apiFetch<Submission[]>("/api/submissions");
        if (!cancelled) setSubmissions(data);
      } catch {}
    };
    load();
    const t = setInterval(load, 3000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, [reload]);
  const load = useCallback(() => setReload((n) => n + 1), []);

  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="mx-auto w-full max-w-6xl px-6 py-8 space-y-6">
        <h1 className="text-2xl font-bold text-zinc-100">Bots</h1>
        <div className="grid gap-6 lg:grid-cols-2">
          <Upload submissions={submissions} onUploaded={load} />
          <BotLeaderboard />
        </div>
        <Submissions submissions={submissions} />
      </main>
    </div>
  );
}

function Upload({ submissions, onUploaded }: { submissions: Submission[] | null; onUploaded: () => void }) {
  const { sessionToken, username } = useAuth();
  const config = useConfig();
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ text: string; error: boolean } | null>(null);
  const pending = submissions?.some((s) => s.username === username && (s.status === "queued" || s.status === "running"));

  const upload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!file) return;
    setBusy(true);
    setMsg(null);
    try {
      const body = new FormData();
      body.append("file", file);
      const res = await fetch(apiUrl("/api/submissions"), {
        method: "POST",
        headers: sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {},
        body,
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || `upload failed (${res.status})`);
      setMsg({ text: "Uploaded, your bot is in the queue", error: false });
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      onUploaded();
    } catch (err) {
      setMsg({ text: err instanceof Error ? err.message : "Upload failed", error: true });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className={`${card} p-6`}>
      <h2 className="text-lg font-semibold">Upload your bot</h2>
      <p className="mt-2 text-sm text-zinc-400 leading-relaxed">
        We run your bot on the server against the simulated market for{" "}
        <span className="text-zinc-200">
          {config ? `${config.backtestDays} trading days of ${config.backtestDayMin} minutes` : "many trading days"}
        </span>
        , much faster than real time, and score it like the live game: mean daily P&L minus the
        standard deviation of daily P&L. Your bot trades alone against the house bots.
      </p>
      <ul className="mt-3 space-y-1 text-xs text-zinc-500 list-disc pl-5">
        <li>Same SDK, no changes needed: subclass <code className="text-zinc-300">Exchange</code> and call <code className="text-zinc-300">.run()</code>. <code className="text-zinc-300">on_tick</code> runs once per simulated second.</li>
        <li>Upload one <code className="text-zinc-300">.py</code> file, or a <code className="text-zinc-300">.zip</code> with <code className="text-zinc-300">bot.py</code> plus your other files (max 5 MB).</li>
        <li>No internet access. numpy, pandas, scipy, scikit-learn and statsmodels are installed.</li>
        <li>Limits: 5 s per <code className="text-zinc-300">on_tick</code>, 100 API calls per tick, 30 min per run, about 250 MB of memory.</li>
        <li>One bot in the queue per team at a time. The leaderboard uses your latest successful run.</li>
      </ul>
      <form onSubmit={upload} className="mt-5 flex flex-wrap items-center gap-3">
        <input
          ref={fileRef}
          type="file"
          accept=".py,.zip"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          className="text-sm text-zinc-300 file:mr-3 file:rounded file:border-0 file:bg-zinc-800 file:px-3 file:py-2 file:text-sm file:text-zinc-200 hover:file:bg-zinc-700"
        />
        <button
          type="submit"
          disabled={!file || busy || pending}
          className="rounded bg-blue-600 px-5 py-2 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {busy ? "Uploading..." : "Submit bot"}
        </button>
        {pending && <span className="text-xs text-zinc-500">Wait for your current bot to finish</span>}
      </form>
      {msg && <p className={`mt-3 text-sm ${msg.error ? "text-red-400" : "text-emerald-400"}`}>{msg.text}</p>}
    </section>
  );
}

function BotLeaderboard() {
  const rows = usePolled<BotLeaderboardEntry[]>("/api/bot-leaderboard", 10000);
  return (
    <section className={`${card} flex flex-col max-h-[520px]`}>
      <div className="px-5 py-4 border-b border-zinc-800/60">
        <h2 className="text-lg font-semibold">Bot leaderboard</h2>
        <p className="mt-1 text-xs text-zinc-500">Each team&apos;s latest successful backtest. Score = mean - std of daily P&L</p>
      </div>
      <div className="overflow-y-auto">
        {rows === null ? (
          <div className="p-8 text-center text-sm text-zinc-500">Loading...</div>
        ) : rows.length === 0 ? (
          <div className="p-8 text-center text-sm text-zinc-500">No bots scored yet</div>
        ) : (
          <table className="w-full text-sm tabular-nums">
            <thead className="text-xs text-zinc-500">
              <tr className="text-left">
                <th className="px-5 py-2 font-medium">#</th>
                <th className="py-2 font-medium">Team</th>
                <th className="py-2 font-medium text-right">Score</th>
                <th className="py-2 font-medium text-right">Mean</th>
                <th className="py-2 font-medium text-right">Std</th>
                <th className="px-5 py-2 font-medium text-right">Days</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800/30">
              {rows.map((r, i) => (
                <tr key={r.username}>
                  <td className="px-5 py-2 text-zinc-500">{i + 1}</td>
                  <td className="py-2">
                    <div className="text-zinc-200">{r.username}</div>
                    <div className="text-[11px] text-zinc-600">{r.filename} · {ago(r.submittedAt)}</div>
                  </td>
                  <td className={`py-2 text-right font-semibold ${pnlClass(r.score)}`}>{formatMoney(r.score, true)}</td>
                  <td className={`py-2 text-right ${pnlClass(r.meanDailyPnl)}`}>{formatMoney(r.meanDailyPnl, true)}</td>
                  <td className="py-2 text-right text-zinc-400">{formatMoney(r.stdDailyPnl)}</td>
                  <td className="px-5 py-2 text-right text-zinc-400">
                    {r.days}
                    {r.busts > 0 && <span className="text-red-400/80"> · {r.busts}b</span>}
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

function StatusBadge({ s }: { s: Submission }) {
  const base = "inline-block rounded-md px-2 py-0.5 text-[11px] font-semibold";
  if (s.status === "queued") return <span className={`${base} bg-zinc-700/40 text-zinc-300`}>queued #{s.queuePosition}</span>;
  if (s.status === "running") return <span className={`${base} bg-blue-500/15 text-blue-400`}>running</span>;
  if (s.status === "error") return <span className={`${base} bg-red-500/15 text-red-400`}>error</span>;
  return <span className={`${base} bg-emerald-500/15 text-emerald-400`}>done</span>;
}

function Submissions({ submissions }: { submissions: Submission[] | null }) {
  const { isAdmin } = useAuth();
  const [open, setOpen] = useState<string | null>(null);

  return (
    <section className={card}>
      <div className="px-5 py-4 border-b border-zinc-800/60">
        <h2 className="text-lg font-semibold">{isAdmin ? "All submissions" : "Your submissions"}</h2>
      </div>
      {submissions === null ? (
        <div className="p-8 text-center text-sm text-zinc-500">Loading...</div>
      ) : submissions.length === 0 ? (
        <div className="p-8 text-center text-sm text-zinc-500">Nothing submitted yet</div>
      ) : (
        <div className="divide-y divide-zinc-800/30">
          {submissions.map((s) => (
            <div key={s.id}>
              <button
                type="button"
                onClick={() => setOpen(open === s.id ? null : s.id)}
                className="w-full flex items-center gap-4 px-5 py-3 text-left text-sm hover:bg-zinc-800/20"
              >
                <StatusBadge s={s} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-zinc-200">
                    {isAdmin && <span className="text-zinc-400">{s.username} · </span>}
                    {s.filename}
                  </div>
                  <div className="text-[11px] text-zinc-600">
                    {ago(s.createdAt)} · {s.params.days} days x {s.params.dayMin} min
                    {isAdmin && s.params.seed !== undefined && ` · seed ${s.params.seed}`}
                    {s.status === "running" && s.startedAt && ` · started ${ago(s.startedAt)}`}
                  </div>
                </div>
                {s.result?.status === "done" && (
                  <div className="text-right tabular-nums">
                    <div className={`font-semibold ${pnlClass(s.result.score)}`}>{formatMoney(s.result.score, true)}</div>
                    <div className="text-[11px] text-zinc-500">
                      mean {formatMoney(s.result.meanDailyPnl, true)} · sd {formatMoney(s.result.stdDailyPnl)}
                    </div>
                  </div>
                )}
                {s.result?.status === "error" && (
                  <div className="max-w-[45%] truncate text-xs text-red-400/80">{s.result.error}</div>
                )}
                <span className="text-zinc-600">{open === s.id ? "▾" : "▸"}</span>
              </button>
              {open === s.id && <SubmissionDetail id={s.id} status={s.status} />}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function SubmissionDetail({ id, status }: { id: string; status: Submission["status"] }) {
  const [sub, setSub] = useState<Submission | null>(null);
  useEffect(() => {
    let cancelled = false;
    apiFetch<Submission>(`/api/submissions/${id}`).then((s) => !cancelled && setSub(s)).catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [id, status]);

  if (!sub) return <div className="px-5 pb-4 text-sm text-zinc-500">Loading...</div>;
  const r = sub.result;
  if (!r) return <div className="px-5 pb-4 text-sm text-zinc-500">Not run yet.</div>;

  return (
    <div className="space-y-4 px-5 pb-5">
      {r.error && <div className="rounded-lg border border-red-500/20 bg-red-500/5 p-3 text-sm text-red-300">{r.error}</div>}
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-zinc-400 tabular-nums">
        <span>{r.days.length} days scored</span>
        <span>{r.fills.toLocaleString()} fills</span>
        <span>{r.volume.toLocaleString()} units traded</span>
        <span>{r.busts} busts</span>
        <span>ran in {r.wallSec}s</span>
      </div>
      {r.days.length > 0 && (
        <div className="h-40">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={r.days.map((d, i) => ({ ...d, n: i + 1 }))}>
              <XAxis dataKey="n" stroke="#52525b" tick={{ fill: "#71717a", fontSize: 11 }} />
              <YAxis stroke="#52525b" tick={{ fill: "#71717a", fontSize: 11 }} width={60} />
              <Tooltip
                contentStyle={{ backgroundColor: "#18181b", border: "1px solid #27272a", borderRadius: 12, fontSize: 12 }}
                labelFormatter={(n) => `Day ${n}`}
                formatter={(v) => [formatMoney(Number(v), true), "P&L"]}
              />
              <Bar dataKey="pnl" isAnimationActive={false}>
                {r.days.map((d, i) => (
                  <Cell key={i} fill={d.pnl >= 0 ? "#34d399" : "#f87171"} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
      <div>
        <div className="mb-1 text-xs text-zinc-500">Bot output (print, self.log, errors)</div>
        <pre className="max-h-80 overflow-auto rounded-lg bg-zinc-950 p-3 text-[11px] leading-relaxed text-zinc-300 whitespace-pre-wrap">
          {r.log ? stripAnsi(r.log) : "(no output)"}
        </pre>
      </div>
    </div>
  );
}
