"use client";

import { useState } from "react";
import {
  useProducts,
  useExchangeOpen,
  useAllUsers,
  useAdminActions,
  useConfig,
  useAccount,
  errorMessage,
  type ConfigUpdate,
  type ExchangeConfig,
} from "@/lib/exchange";
import { useAuth } from "@/lib/auth";
import Header from "@/components/Header";

type EtfComponentInput = {
  productId: string;
  weight: string;
};

export default function AdminPage() {
  const { isAdmin } = useAuth();

  if (!isAdmin) {
    return (
      <div className="min-h-screen flex flex-col">
        <Header />
        <div className="mx-auto max-w-3xl px-6 py-16 text-center">
          <h1 className="text-2xl font-semibold text-zinc-100">Access denied</h1>
          <p className="mt-2 text-zinc-400">You must be an admin to view this page.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <div className="mx-auto max-w-6xl px-6 py-8 space-y-6">
        <h1 className="text-2xl font-bold text-zinc-100">Admin</h1>
        <ExchangeControl />
        <ProjectConfigSection />
        <ProductsSection />
        <EtfCreationSection />
        <UsersSection />
      </div>
    </div>
  );
}

function ExchangeControl() {
  const isOpen = useExchangeOpen();
  const admin = useAdminActions();
  const [busy, setBusy] = useState(false);

  const handleToggle = async () => {
    setBusy(true);
    try {
      await admin.toggleExchange();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Toggle failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="rounded-2xl border border-zinc-800/60 bg-zinc-900/50 p-6">
      <h2 className="mb-4 text-lg font-semibold text-zinc-100">Exchange Control</h2>
      <div className="flex items-center gap-4">
        <span
          className={`inline-block h-3 w-3 rounded-full ${
            isOpen ? "bg-green-500" : "bg-red-500"
          }`}
        />
        <span className="text-zinc-300">
          Status:{" "}
          <span className={isOpen ? "text-green-400" : "text-red-400"}>
            {isOpen ? "OPEN" : "CLOSED"}
          </span>
        </span>
        <button
          onClick={handleToggle}
          disabled={busy}
          className={`ml-auto rounded px-6 py-2 font-semibold text-white disabled:opacity-50 ${
            isOpen
              ? "bg-red-600 hover:bg-red-700"
              : "bg-green-600 hover:bg-green-700"
          }`}
        >
          {isOpen ? "Stop Exchange" : "Open Exchange"}
        </button>
      </div>
    </section>
  );
}

function ProductsSection() {
  const products = useProducts();
  const admin = useAdminActions();

  const [symbol, setSymbol] = useState("");
  const [name, setName] = useState("");
  const [positionLimit, setPositionLimit] = useState("100");
  const [creating, setCreating] = useState(false);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const limit = Number(positionLimit);
    if (!symbol.trim() || !name.trim() || !Number.isFinite(limit)) {
      alert("Symbol, name, and position limit are required");
      return;
    }
    setCreating(true);
    try {
      await admin.createProduct(
        symbol.trim().toUpperCase(),
        name.trim(),
        limit,
      );
      setSymbol("");
      setName("");
      setPositionLimit("100");
    } catch (err) {
      alert(err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (productId: string, symbol: string) => {
    if (!window.confirm(`Delete product ${symbol}?`)) return;
    try {
      await admin.deleteProduct(productId);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Delete failed");
    }
  };

  const handleLimitChange = async (productId: string, value: string) => {
    const limit = Number(value);
    if (!Number.isFinite(limit)) return;
    try {
      await admin.updatePositionLimit(productId, limit);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Update failed");
    }
  };

  const handleTickSizeChange = async (productId: string, value: string) => {
    const tick = Number(value);
    if (!Number.isFinite(tick) || tick <= 0) return;
    try {
      await admin.updateTickSize(productId, tick);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Update failed");
    }
  };

  return (
    <section className="rounded-2xl border border-zinc-800/60 bg-zinc-900/50 p-6">
      <h2 className="mb-4 text-lg font-semibold text-zinc-100">Products</h2>
      <div className="overflow-x-auto">
        <table className="w-full divide-y divide-zinc-800 text-sm">
          <thead>
            <tr className="text-left text-zinc-400">
              <th className="pb-2 pr-4">Symbol</th>
              <th className="pb-2 pr-4">Name</th>
              <th className="pb-2 pr-4">Type</th>
              <th className="pb-2 pr-4">Position Limit</th>
              <th className="pb-2 pr-4">Tick Size</th>
              <th className="pb-2">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800">
            {products.map((p) => (
              <tr key={p.id} className="text-zinc-200">
                <td className="py-2 pr-4 font-mono">{p.symbol}</td>
                <td className="py-2 pr-4">{p.name}</td>
                <td className="py-2 pr-4">
                  <span
                    className={`rounded px-2 py-0.5 text-xs ${
                      p.kind === "etf"
                        ? "bg-purple-900 text-purple-200"
                        : p.kind === "option"
                          ? "bg-sky-900 text-sky-200"
                          : "bg-blue-900 text-blue-200"
                    }`}
                  >
                    {p.kind === "etf" ? "ETF" : p.kind === "option" ? "Option" : "Fruit"}
                  </span>
                </td>
                <td className="py-2 pr-4">
                  <PositionLimitInput
                    initial={p.positionLimit}
                    onCommit={(v) => handleLimitChange(p.id, v)}
                  />
                </td>
                <td className="py-2 pr-4">
                  <PositionLimitInput
                    initial={p.tickSize ?? 1}
                    onCommit={(v) => handleTickSizeChange(p.id, v)}
                    step="any"
                  />
                </td>
                <td className="py-2">
                  <button
                    onClick={() => handleDelete(p.id, p.symbol)}
                    className="rounded bg-red-600 px-3 py-1 text-xs text-white hover:bg-red-700"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
            {products.length === 0 && (
              <tr>
                <td colSpan={6} className="py-4 text-center text-zinc-500">
                  No products
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <form onSubmit={handleCreate} className="mt-6 border-t border-zinc-800 pt-6">
        <h3 className="mb-3 text-sm font-semibold text-zinc-300">Add Product</h3>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
          <input
            type="text"
            placeholder="Symbol (e.g. AAPL)"
            value={symbol}
            onChange={(e) => setSymbol(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
          />
          <input
            type="text"
            placeholder="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
          />
          <input
            type="number"
            placeholder="Position limit"
            value={positionLimit}
            onChange={(e) => setPositionLimit(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
          />
          <button
            type="submit"
            disabled={creating}
            className="rounded bg-blue-600 px-4 py-2 font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {creating ? "Creating..." : "Add Product"}
          </button>
        </div>
      </form>
    </section>
  );
}

function PositionLimitInput({
  initial,
  onCommit,
  step,
}: {
  initial: number;
  onCommit: (value: string) => void;
  step?: string;
}) {
  const [value, setValue] = useState(String(initial));

  return (
    <input
      type="number"
      step={step}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={() => {
        if (value !== String(initial)) onCommit(value);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
      }}
      className="w-28 rounded border border-zinc-700 bg-zinc-800 px-2 py-1 text-zinc-100"
    />
  );
}

function EtfCreationSection() {
  const products = useProducts();
  const admin = useAdminActions();

  const [symbol, setSymbol] = useState("");
  const [name, setName] = useState("");
  const [positionLimit, setPositionLimit] = useState("100");
  const emptyComp = (): EtfComponentInput[] => [{ productId: "", weight: "1" }];
  const [compositions, setCompositions] = useState<EtfComponentInput[][]>([emptyComp()]);
  const [creating, setCreating] = useState(false);

  const updateComponent = (compIdx: number, rowIdx: number, patch: Partial<EtfComponentInput>) => {
    setCompositions((prev) =>
      prev.map((comp, ci) =>
        ci === compIdx ? comp.map((c, ri) => (ri === rowIdx ? { ...c, ...patch } : c)) : comp
      )
    );
  };

  const addComponent = (compIdx: number) => {
    setCompositions((prev) =>
      prev.map((comp, ci) => (ci === compIdx ? [...comp, { productId: "", weight: "1" }] : comp))
    );
  };

  const removeComponent = (compIdx: number, rowIdx: number) => {
    setCompositions((prev) =>
      prev.map((comp, ci) => (ci === compIdx ? comp.filter((_, ri) => ri !== rowIdx) : comp))
    );
  };

  const addComposition = () => {
    setCompositions((prev) => [...prev, emptyComp()]);
  };

  const removeComposition = (compIdx: number) => {
    setCompositions((prev) => prev.filter((_, ci) => ci !== compIdx));
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const limit = Number(positionLimit);
    if (!symbol.trim() || !name.trim() || !Number.isFinite(limit)) {
      alert("Symbol, name, and position limit are required");
      return;
    }
    const parsed = compositions.map((comp) =>
      comp
        .filter((c) => c.productId)
        .map((c) => ({ productId: c.productId, weight: Number(c.weight) }))
    ).filter((comp) => comp.length > 0);
    if (parsed.length === 0) {
      alert("ETF must have at least one composition with at least one component");
      return;
    }
    if (parsed.some((comp) => comp.some((c) => !Number.isFinite(c.weight)))) {
      alert("Invalid weight value");
      return;
    }
    setCreating(true);
    try {
      await admin.createEtf(symbol.trim().toUpperCase(), name.trim(), limit, parsed);
      setSymbol("");
      setName("");
      setPositionLimit("100");
      setCompositions([emptyComp()]);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Create ETF failed");
    } finally {
      setCreating(false);
    }
  };

  return (
    <section className="rounded-2xl border border-zinc-800/60 bg-zinc-900/50 p-6">
      <h2 className="mb-4 text-lg font-semibold text-zinc-100">Create ETF</h2>
      <form onSubmit={handleCreate} className="space-y-4">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          <input
            type="text"
            placeholder="Symbol"
            value={symbol}
            onChange={(e) => setSymbol(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
          />
          <input
            type="text"
            placeholder="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
          />
          <input
            type="number"
            placeholder="Position limit"
            value={positionLimit}
            onChange={(e) => setPositionLimit(e.target.value)}
            className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
          />
        </div>

        <div className="space-y-4">
          {compositions.map((comp, ci) => (
            <div key={ci} className="rounded-lg border border-zinc-700/50 bg-zinc-800/30 p-3">
              <div className="mb-2 flex items-center justify-between">
                <h3 className="text-sm font-semibold text-zinc-300">
                  Composition {compositions.length > 1 ? ci + 1 : ""}
                </h3>
                {compositions.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeComposition(ci)}
                    className="rounded bg-red-600/80 px-2 py-0.5 text-xs text-white hover:bg-red-700"
                  >
                    Remove
                  </button>
                )}
              </div>
              <div className="space-y-2">
                {comp.map((c, ri) => (
                  <div key={ri} className="flex gap-2">
                    <select
                      value={c.productId}
                      onChange={(e) => updateComponent(ci, ri, { productId: e.target.value })}
                      className="flex-1 rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100"
                    >
                      <option value="">Select product...</option>
                      {products.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.symbol}{p.isEtf ? " (ETF)" : ""} - {p.name}
                        </option>
                      ))}
                    </select>
                    <input
                      type="number"
                      step="any"
                      placeholder="Weight"
                      value={c.weight}
                      onChange={(e) => updateComponent(ci, ri, { weight: e.target.value })}
                      className="w-32 rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-zinc-100 placeholder-zinc-500"
                    />
                    <button
                      type="button"
                      onClick={() => removeComponent(ci, ri)}
                      disabled={comp.length <= 1}
                      className="rounded bg-zinc-700 px-3 py-2 text-zinc-200 hover:bg-zinc-600 disabled:opacity-40"
                    >
                      Remove
                    </button>
                  </div>
                ))}
              </div>
              <button
                type="button"
                onClick={() => addComponent(ci)}
                className="mt-2 rounded bg-zinc-700 px-3 py-1 text-sm text-zinc-200 hover:bg-zinc-600"
              >
                + Add Component
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addComposition}
            className="rounded border border-dashed border-zinc-600 bg-zinc-800/30 px-4 py-2 text-sm text-zinc-400 hover:border-zinc-500 hover:text-zinc-300"
          >
            + Add Alternative Composition
          </button>
        </div>

        <button
          type="submit"
          disabled={creating}
          className="rounded bg-blue-600 px-4 py-2 font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {creating ? "Creating..." : "Create ETF"}
        </button>
      </form>
    </section>
  );
}

function UsersSection() {
  const users = useAllUsers();
  const admin = useAdminActions();

  const handleResetUser = async (userId: string, username: string) => {
    if (!window.confirm(`Reset user ${username}? This will clear all their orders, fills, positions, and PnL.`)) {
      return;
    }
    try {
      await admin.resetUser(userId);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Reset failed");
    }
  };

  const handleTogglePosLimit = async (userId: string, current: boolean) => {
    try {
      await admin.setNoPositionLimit(userId, !current);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Toggle failed");
    }
  };

  const handleRateLimitChange = async (userId: string, value: string) => {
    const ms = Number(value);
    if (!Number.isFinite(ms) || ms < 0) return;
    try {
      await admin.setRateLimit(userId, ms);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Update failed");
    }
  };

  const handleResetAll = async () => {
    if (!window.confirm("RESET ALL DATA? This will wipe all orders, fills, positions, and PnL for every user. This cannot be undone.")) {
      return;
    }
    if (!window.confirm("Are you absolutely sure? Last chance to cancel.")) {
      return;
    }
    try {
      await admin.resetAll();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Reset all failed");
    }
  };

  return (
    <section className="rounded-2xl border border-zinc-800/60 bg-zinc-900/50 p-6">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">Users</h2>
        <button
          onClick={handleResetAll}
          className="rounded bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700"
        >
          Reset ALL
        </button>
      </div>
      <p className="mb-4 text-xs text-red-400">
        Warning: reset operations permanently delete orders, fills, positions, and PnL data.
      </p>
      <div className="overflow-x-auto">
        <table className="w-full divide-y divide-zinc-800 text-sm">
          <thead>
            <tr className="text-left text-zinc-400">
              <th className="pb-2 pr-4">Username</th>
              <th className="pb-2 pr-4">Admin</th>
              <th className="pb-2 pr-4">Pos Limit</th>
              <th className="pb-2 pr-4">Rate Limit (ms)</th>
              <th className="pb-2">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800">
            {users?.slice().sort((a, b) => a.username.localeCompare(b.username)).map((u) => (
              <tr key={u._id} className="text-zinc-200">
                <td className="py-2 pr-4 font-mono">{u.username}</td>
                <td className="py-2 pr-4">{u.isAdmin ? "Yes" : "No"}</td>
                <td className="py-2 pr-4">
                  <button
                    onClick={() => handleTogglePosLimit(u._id, !!u.noPositionLimit)}
                    className={`rounded px-3 py-1 text-xs font-medium ${
                      u.noPositionLimit
                        ? "bg-amber-600 text-white hover:bg-amber-700"
                        : "bg-zinc-700 text-zinc-400 hover:bg-zinc-600"
                    }`}
                  >
                    {u.noPositionLimit ? "Unlimited" : "Normal"}
                  </button>
                </td>
                <td className="py-2 pr-4">
                  <PositionLimitInput
                    initial={u.rateLimitMs ?? 10}
                    onCommit={(v) => handleRateLimitChange(u._id, v)}
                  />
                </td>
                <td className="py-2">
                  <button
                    onClick={() => handleResetUser(u._id, u.username)}
                    className="rounded bg-red-600 px-3 py-1 text-xs text-white hover:bg-red-700"
                  >
                    Reset
                  </button>
                </td>
              </tr>
            ))}
            {users && users.length === 0 && (
              <tr>
                <td colSpan={5} className="py-4 text-center text-zinc-500">
                  No users
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ProjectConfigSection() {
  const config = useConfig();
  if (!config) {
    return (
      <section className="rounded-2xl border border-zinc-800/60 bg-zinc-900/50 p-6 text-zinc-500 text-sm">
        Loading config...
      </section>
    );
  }
  // Remount the form whenever the server config changes so inputs show the live values.
  return <ProjectConfigForm key={JSON.stringify(config)} config={config} />;
}

function ProjectConfigForm({ config }: { config: ExchangeConfig }) {
  const admin = useAdminActions();
  const account = useAccount();
  const [form, setForm] = useState({
    phase: config.phase,
    startingCash: String(config.startingCash),
    dayLengthMin: String(config.dayLengthMin),
    etfFee: String(config.etfFee),
    fruitMarginRate: String(config.marginRates.fruit),
    etfMarginRate: String(config.marginRates.etf),
    expiries: new Set(config.enabledExpiries),
    backtestDays: String(config.backtestDays),
    backtestDayMin: String(config.backtestDayMin),
    backtestSeed: String(config.backtestSeed ?? 0),
  });
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ text: string; error: boolean } | null>(null);

  const flash = (text: string, error = false) => {
    setMsg({ text, error });
    setTimeout(() => setMsg(null), 4000);
  };

  const apply = async (update: ConfigUpdate, ok = "Saved") => {
    setBusy(true);
    try {
      await admin.updateConfig(update);
      flash(ok);
    } catch (err) {
      flash(errorMessage(err, "Update failed"), true);
    } finally {
      setBusy(false);
    }
  };

  const save = (e: React.FormEvent) => {
    e.preventDefault();
    const nums = {
      startingCash: Number(form.startingCash),
      dayLengthMin: Number(form.dayLengthMin),
      etfFee: Number(form.etfFee),
      fruitMarginRate: Number(form.fruitMarginRate),
      etfMarginRate: Number(form.etfMarginRate),
      backtestDays: Number(form.backtestDays),
      backtestDayMin: Number(form.backtestDayMin),
      backtestSeed: Number(form.backtestSeed),
    };
    if (Object.values(nums).some((v) => !Number.isFinite(v))) {
      flash("All fields must be numbers", true);
      return;
    }
    const enabled = [...form.expiries].sort((a, b) => a - b);
    if (enabled.join() !== config.enabledExpiries.join() &&
        !window.confirm("Disabling an option expiry cancels its orders and refunds open positions at entry price. Continue?")) {
      return;
    }
    apply({ ...nums, phase: form.phase, enabledExpiries: enabled });
  };

  const endDay = async () => {
    if (!window.confirm("End the trading day now? Options settle, everyone's P&L is recorded as a daily result, and all accounts reset to starting cash.")) return;
    setBusy(true);
    try {
      await admin.endDay();
      flash("Day ended");
    } catch (err) {
      flash(errorMessage(err, "End day failed"), true);
    } finally {
      setBusy(false);
    }
  };

  const rerunBots = async () => {
    if (!window.confirm("Backtest every team's latest bot again with the current backtest settings? They run one at a time.")) return;
    setBusy(true);
    try {
      const r = await admin.rerunBots();
      flash(`Queued ${r.queued} bot${r.queued === 1 ? "" : "s"}`);
    } catch (err) {
      flash(errorMessage(err, "Re-run failed"), true);
    } finally {
      setBusy(false);
    }
  };

  const input = "w-full rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 tabular-nums";
  const field = (label: string, key: keyof typeof form, hint?: string) => (
    <label className="flex flex-col gap-1">
      <span className="text-xs text-zinc-400">{label}</span>
      <input
        className={input}
        value={form[key] as string}
        onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))}
      />
      {hint && <span className="text-[11px] text-zinc-600">{hint}</span>}
    </label>
  );

  return (
    <section className="rounded-2xl border border-zinc-800/60 bg-zinc-900/50 p-6">
      <div className="mb-4 flex items-center gap-3">
        <h2 className="text-lg font-semibold text-zinc-100">Competition Rules</h2>
        {account && (
          <span className="text-xs text-zinc-500">
            Current day: {account.day} · ends {new Date(account.dayEnd).toLocaleString()}
          </span>
        )}
      </div>

      <form onSubmit={save} className="space-y-5">
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
          <label className="flex flex-col gap-1">
            <span className="text-xs text-zinc-400">Phase</span>
            <select
              className={input}
              value={form.phase}
              onChange={(e) => setForm((f) => ({ ...f, phase: e.target.value as "training" | "testing" }))}
            >
              <option value="training">Training</option>
              <option value="testing">Testing</option>
            </select>
          </label>
          {field("Starting cash ($)", "startingCash", "Applies from the next reset")}
          {field("Day length (minutes)", "dayLengthMin", "1440 = UTC midnight to midnight")}
          {field("ETF create/redeem fee ($/unit)", "etfFee")}
          {field("Fruit margin rate", "fruitMarginRate", "Fraction of notional, e.g. 0.2")}
          {field("ETF margin rate", "etfMarginRate")}
        </div>

        <div>
          <span className="text-xs text-zinc-400">Uploaded bot backtests (Bots page)</span>
          <div className="mt-2 grid grid-cols-2 gap-4 md:grid-cols-3">
            {field("Simulated days", "backtestDays", "Score = mean - std of these days")}
            {field("Minutes per simulated day", "backtestDayMin", "Runtime grows with days x minutes")}
            {field("Market seed", "backtestSeed", "Same seed = same fair value paths for every team. Hidden from teams")}
          </div>
          <p className="mt-2 text-[11px] text-zinc-600">
            New settings apply to new uploads. Use &quot;Re-run all bots&quot; to rescore every team&apos;s latest bot (e.g. with a fresh seed for the final ranking).
          </p>
        </div>

        <div>
          <span className="text-xs text-zinc-400">Up-down option expiries</span>
          <div className="mt-2 flex gap-4">
            {[5, 15, 60].map((e) => (
              <label key={e} className="flex items-center gap-2 text-sm text-zinc-300">
                <input
                  type="checkbox"
                  checked={form.expiries.has(e)}
                  onChange={(ev) =>
                    setForm((f) => {
                      const next = new Set(f.expiries);
                      if (ev.target.checked) next.add(e); else next.delete(e);
                      return { ...f, expiries: next };
                    })
                  }
                />
                up{e} {e === 15 && <span className="text-xs text-zinc-500">(settles on ETF mid)</span>}
              </label>
            ))}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3 border-t border-zinc-800/60 pt-4">
          <button
            type="submit"
            disabled={busy}
            className="rounded bg-blue-600 px-5 py-2 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Save rules
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => apply({ simEnabled: !config.simEnabled }, config.simEnabled ? "House bots stopped" : "House bots started")}
            className={`rounded px-5 py-2 text-sm font-semibold disabled:opacity-50 ${
              config.simEnabled
                ? "bg-zinc-800 text-zinc-200 hover:bg-zinc-700"
                : "bg-emerald-600 text-white hover:bg-emerald-700"
            }`}
          >
            {config.simEnabled ? "Stop house bots" : "Start house bots"}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => apply({ clickTrading: !config.clickTrading }, config.clickTrading ? "Click trading disabled" : "Click trading enabled")}
            className={`rounded px-5 py-2 text-sm font-semibold disabled:opacity-50 ${
              config.clickTrading
                ? "bg-zinc-800 text-zinc-200 hover:bg-zinc-700"
                : "bg-emerald-600 text-white hover:bg-emerald-700"
            }`}
            title="Clicking a price level in the web UI sends an IOC order. Bots are unaffected."
          >
            {config.clickTrading ? "Disable click trading" : "Enable click trading"}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={rerunBots}
            className="rounded bg-zinc-800 px-5 py-2 text-sm font-semibold text-zinc-200 hover:bg-zinc-700 disabled:opacity-50"
          >
            Re-run all bots
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={endDay}
            className="ml-auto rounded bg-amber-600 px-5 py-2 text-sm font-semibold text-white hover:bg-amber-700 disabled:opacity-50"
          >
            End day now
          </button>
          {msg && <span className={`text-sm ${msg.error ? "text-red-400" : "text-emerald-400"}`}>{msg.text}</span>}
        </div>
      </form>
    </section>
  );
}
