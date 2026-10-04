"use client";

import { useState } from "react";
import {
  useAccount,
  useConfig,
  useEtfSwap,
  useNowSec,
  usePositions,
  errorMessage,
  formatDuration,
  formatMoney,
} from "@/lib/exchange";

function pnlClass(v: number) {
  if (v > 0) return "text-emerald-400";
  if (v < 0) return "text-red-400";
  return "text-zinc-400";
}

export function AccountPanel() {
  const account = useAccount();
  const config = useConfig();
  const now = useNowSec();

  if (!account) {
    return <section className="px-3 py-3 text-xs text-zinc-600 border-b border-zinc-800/60">Loading account...</section>;
  }

  const marginPct = account.equity > 0 ? Math.min(100, (account.marginUsed / account.equity) * 100) : 100;
  const barColor = marginPct > 85 ? "bg-red-500" : marginPct > 60 ? "bg-amber-500" : "bg-emerald-500";
  const dayLeft = account.dayEnd / 1000 - now;

  return (
    <section className="border-b border-zinc-800/60">
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-zinc-800/60">
        <h2 className="text-xs font-semibold text-zinc-400">Account</h2>
        <span className="text-[10px] text-zinc-500 tabular-nums">
          {config?.phase === "testing" ? "Testing" : "Training"} · {account.day} · ends in {formatDuration(dayLeft)}
        </span>
      </div>

      <div className="px-3 py-2 grid grid-cols-3 gap-x-3 gap-y-1.5 text-[11px] tabular-nums">
        <Stat label="P&L today" value={formatMoney(account.pnl, true)} className={`text-sm font-bold ${pnlClass(account.pnl)}`} />
        <Stat label="Equity" value={formatMoney(account.equity)} />
        <Stat label="Cash" value={formatMoney(account.cash)} />
        <Stat label="Margin used" value={formatMoney(account.marginUsed)} />
        <Stat
          label="Margin free"
          value={formatMoney(account.marginAvailable)}
          className={account.marginAvailable < 0 ? "text-red-400" : "text-zinc-200"}
        />
        <Stat
          label="Busts today"
          value={String(account.busts)}
          className={account.busts > 0 ? "text-red-400 font-semibold" : "text-zinc-200"}
        />
      </div>

      <div className="px-3 pb-2.5">
        <div className="h-1 rounded-full bg-zinc-800 overflow-hidden">
          <div className={`h-full ${barColor} transition-all`} style={{ width: `${marginPct}%` }} />
        </div>
      </div>

      <EtfSwap fee={config?.etfFee ?? 0} />
    </section>
  );
}

function Stat({ label, value, className = "text-zinc-200" }: { label: string; value: string; className?: string }) {
  return (
    <div className="flex flex-col min-w-0">
      <span className="text-[10px] text-zinc-500">{label}</span>
      <span className={`truncate ${className}`}>{value}</span>
    </div>
  );
}

function EtfSwap({ fee }: { fee: number }) {
  const swap = useEtfSwap();
  const positions = usePositions();
  const [qty, setQty] = useState("1");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ text: string; error: boolean } | null>(null);

  const etfPos = positions.find((p) => p.symbol === "etf")?.qty ?? 0;

  const run = async (direction: "create" | "redeem") => {
    const n = Math.floor(Number(qty));
    if (!Number.isFinite(n) || n <= 0) return;
    setBusy(true);
    try {
      const res = await swap("etf", n, direction);
      setMsg({ text: `${direction === "create" ? "Created" : "Redeemed"} ${res.qty} · fee ${formatMoney(res.fee)}`, error: false });
    } catch (err) {
      setMsg({ text: errorMessage(err, "Swap failed"), error: true });
    } finally {
      setBusy(false);
      setTimeout(() => setMsg(null), 4000);
    }
  };

  return (
    <div className="px-3 pb-3">
      <div className="flex items-center gap-1.5">
        <span className="text-[10px] text-zinc-500 mr-auto" title="Create: 1 of each fruit -> 1 ETF. Redeem: 1 ETF -> 1 of each fruit.">
          ETF swap · fee {formatMoney(fee)}/unit · pos {etfPos}
        </span>
        <input
          type="number"
          min={1}
          value={qty}
          onChange={(e) => setQty(e.target.value)}
          className="w-14 px-1.5 py-0.5 text-xs tabular-nums text-center bg-zinc-800 border border-zinc-700/60 rounded-md text-zinc-200 focus:outline-none focus:border-zinc-500"
        />
        <button
          type="button"
          disabled={busy}
          onClick={() => run("create")}
          className="px-2 py-0.5 text-[10px] font-medium rounded-md bg-violet-500/15 text-violet-300 border border-violet-500/25 hover:bg-violet-500/25 disabled:opacity-40"
        >
          Create
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={() => run("redeem")}
          className="px-2 py-0.5 text-[10px] font-medium rounded-md bg-zinc-800 text-zinc-300 border border-zinc-700/60 hover:bg-zinc-700 disabled:opacity-40"
        >
          Redeem
        </button>
      </div>
      {msg && <div className={`mt-1.5 text-[10px] ${msg.error ? "text-red-400" : "text-zinc-400"}`}>{msg.text}</div>}
    </div>
  );
}
