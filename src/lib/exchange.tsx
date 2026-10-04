"use client";

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
  useSyncExternalStore,
  ReactNode,
} from "react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ProductKind = "fruit" | "etf" | "option";

export interface OptionInfo {
  expiryMin: number;
  windowOpen: number; // epoch seconds
  windowClose: number; // epoch seconds
  strike: number | null;
  priceSource: "etf_last_trade" | "etf_mid";
}

export interface Product {
  id: string;
  symbol: string;
  name: string;
  isEtf: boolean;
  kind: ProductKind;
  compositions?: Array<Array<{ symbol: string; weight: number }>>;
  positionLimit: number;
  tickSize: number;
  option?: OptionInfo;
}

export interface Account {
  cash: number;
  equity: number;
  pnl: number;
  startingCash: number;
  marginUsed: number;
  marginAvailable: number;
  busts: number;
  day: string;
  dayStart: number; // ms
  dayEnd: number; // ms
}

export interface ExchangeConfig {
  phase: "training" | "testing";
  startingCash: number;
  dayLengthMin: number;
  etfFee: number;
  marginRates: { fruit: number; etf: number };
  enabledExpiries: number[];
  clickTrading: boolean; // click a book level to trade in the web UI
  simEnabled?: boolean; // admin only
}

export interface OptionWindow {
  expiryMin: number;
  windowOpen: number;
  strike: number;
  settle: number;
  upWon: boolean;
  priceSource: string;
}

export interface DailyResult {
  day: string;
  pnl: number;
  busts: number;
  endedAt: number;
}

export interface DataFile {
  table: string;
  date: string;
  path: string;
  bytes: number;
}

export interface BookLevel {
  price: number;
  qty: number;
  orderCount: number;
}

export interface BookSnapshot {
  bids: BookLevel[];
  asks: BookLevel[];
  lastTradePrice: number | null;
}

export interface Position {
  productId: string;
  symbol: string;
  qty: number;
  realizedPnl: number;
  avgEntryPrice: number;
  unrealizedPnl: number;
}

export interface OpenOrder {
  _id: string;
  productId: string;
  symbol: string;
  side: string;
  price: number;
  qty: number;
  filledQty: number;
  status: string;
  orderType: string;
  createdAt: number;
}

export interface Trade {
  _id: string;
  symbol: string;
  price: number;
  qty: number;
  createdAt: number;
  isMine: boolean;
  mySide: string | null;
  buyer: string;
  seller: string;
}

export interface LeaderboardEntry {
  userId: string;
  username: string;
  totalPnl: number;
  realizedPnl: number;
  unrealizedPnl: number;
  days: number;
  meanDailyPnl: number;
  stdDailyPnl: number;
  score: number;
  busts: number;
}

export interface AllPosition {
  username: string;
  symbol: string;
  qty: number;
  avgEntryPrice: number;
  realizedPnl: number;
  unrealizedPnl: number;
}

export interface UserEntry {
  _id: string;
  username: string;
  isAdmin: boolean;
  noPositionLimit?: boolean;
  rateLimitMs?: number;
}

export interface ExchangeState {
  exchangeOpen: boolean;
  serverTime: number; // ms
  account: Account | null;
  config: ExchangeConfig | null;
  optionWindows: OptionWindow[];
  products: Product[];
  books: Record<string, BookSnapshot>;
  positions: Position[];
  openOrders: OpenOrder[];
  pnl: number;
  recentTrades: Trade[];
  leaderboard: LeaderboardEntry[];
  allPositions: AllPosition[];
  allUsers?: UserEntry[];
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const BASE_URL =
  process.env.NEXT_PUBLIC_EXCHANGE_URL ?? (typeof window !== "undefined" ? "" : "http://localhost:3211");

function wsUrl(): string {
  // Same-origin deploy (no NEXT_PUBLIC_EXCHANGE_URL): derive ws:// or wss:// from the page.
  if (!BASE_URL && typeof window !== "undefined") {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${window.location.host}/ws`;
  }
  return `${BASE_URL.replace(/^http/, "ws")}/ws`;
}

export function apiUrl(path: string): string {
  return `${BASE_URL}${path}`;
}

/** Decimal places implied by a tick size (0.01 -> 2). */
export function tickDecimals(tickSize: number | undefined): number {
  if (!tickSize || tickSize >= 1) return 0;
  return Math.min(8, Math.max(0, Math.round(-Math.log10(tickSize))));
}

export function formatPrice(price: number | null | undefined, tickSize = 0.01): string {
  if (price === null || price === undefined || !Number.isFinite(price)) return "-";
  return price.toFixed(tickDecimals(tickSize));
}

export function formatMoney(v: number, signed = false): string {
  const s = Math.abs(v).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  if (v < 0) return `-${s}`;
  return signed && v > 0 ? `+${s}` : s;
}

export function formatDuration(sec: number): string {
  sec = Math.max(0, Math.floor(sec));
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

function getSessionToken(): string | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem("exchange_auth");
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    return parsed.sessionToken ?? null;
  } catch {
    return null;
  }
}

export async function apiFetch<T = unknown>(
  path: string,
  opts: RequestInit = {},
): Promise<T> {
  const token = getSessionToken();
  const res = await fetch(apiUrl(path), {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(opts.headers as Record<string, string> | undefined),
    },
  });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`API ${res.status}: ${body}`);
  }
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

// ---------------------------------------------------------------------------
// Structural sharing helpers
// ---------------------------------------------------------------------------

function jsonEq(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

function mergeBooks(
  prev: Record<string, BookSnapshot>,
  next: Record<string, BookSnapshot>,
): Record<string, BookSnapshot> {
  const nextKeys = Object.keys(next);
  const prevKeys = Object.keys(prev);
  if (nextKeys.length !== prevKeys.length) return next;

  let allSame = true;
  const merged: Record<string, BookSnapshot> = {};
  for (const k of nextKeys) {
    if (prev[k] && jsonEq(prev[k], next[k])) {
      merged[k] = prev[k];
    } else {
      merged[k] = next[k];
      allSame = false;
    }
  }
  return allSame ? prev : merged;
}

function mergeState(prev: ExchangeState, next: ExchangeState): ExchangeState {
  const books = mergeBooks(prev.books, next.books);
  const products = jsonEq(prev.products, next.products) ? prev.products : next.products;
  const positions = jsonEq(prev.positions, next.positions) ? prev.positions : next.positions;
  const openOrders = jsonEq(prev.openOrders, next.openOrders) ? prev.openOrders : next.openOrders;
  const recentTrades = jsonEq(prev.recentTrades, next.recentTrades) ? prev.recentTrades : next.recentTrades;
  const leaderboard = jsonEq(prev.leaderboard, next.leaderboard) ? prev.leaderboard : next.leaderboard;
  const allPositions = jsonEq(prev.allPositions, next.allPositions) ? prev.allPositions : next.allPositions;
  const allUsers = jsonEq(prev.allUsers, next.allUsers) ? prev.allUsers : next.allUsers;
  const account = jsonEq(prev.account, next.account) ? prev.account : next.account;
  const config = jsonEq(prev.config, next.config) ? prev.config : next.config;
  const optionWindows = jsonEq(prev.optionWindows, next.optionWindows) ? prev.optionWindows : next.optionWindows;

  if (
    books === prev.books &&
    products === prev.products &&
    positions === prev.positions &&
    openOrders === prev.openOrders &&
    recentTrades === prev.recentTrades &&
    leaderboard === prev.leaderboard &&
    allPositions === prev.allPositions &&
    allUsers === prev.allUsers &&
    account === prev.account &&
    config === prev.config &&
    optionWindows === prev.optionWindows &&
    next.pnl === prev.pnl &&
    next.exchangeOpen === prev.exchangeOpen
  ) {
    return prev;
  }

  return {
    exchangeOpen: next.exchangeOpen,
    serverTime: next.serverTime,
    account,
    config,
    optionWindows,
    products,
    books,
    positions,
    openOrders,
    pnl: next.pnl,
    recentTrades,
    leaderboard,
    allPositions,
    allUsers,
  };
}

// ---------------------------------------------------------------------------
// Exchange store - external store for useSyncExternalStore
// Components only re-render when their specific selector output changes.
// ---------------------------------------------------------------------------

type Listener = () => void;

class ExchangeStore {
  private state: ExchangeState | null = null;
  private listeners = new Set<Listener>();
  private pending: ExchangeState | null = null;
  private flushTimer: ReturnType<typeof setInterval> | null = null;
  private ws: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private mounted = false;

  start() {
    this.mounted = true;
    this.flushTimer = setInterval(() => this.flush(), 100);
    this.connect();

    const onStorage = () => {
      const token = getSessionToken();
      if (!token) {
        this.ws?.close();
        this.ws = null;
        this.pending = null;
        this.setState(null);
      } else if (!this.ws || this.ws.readyState > WebSocket.OPEN) {
        this.connect();
      }
    };
    window.addEventListener("storage", onStorage);

    const tokenPoll = setInterval(() => {
      const token = getSessionToken();
      if (!token && this.ws) {
        this.ws.close();
        this.ws = null;
        this.pending = null;
        this.setState(null);
      } else if (token && (!this.ws || this.ws.readyState > WebSocket.OPEN)) {
        this.connect();
      }
    }, 2000);

    return () => {
      this.mounted = false;
      window.removeEventListener("storage", onStorage);
      if (this.flushTimer) clearInterval(this.flushTimer);
      clearInterval(tokenPoll);
      if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
      this.ws?.close();
      this.ws = null;
    };
  }

  private connect() {
    const token = getSessionToken();
    if (!token) return;

    const ws = new WebSocket(wsUrl());
    this.ws = ws;

    ws.onopen = () => {
      ws.send(JSON.stringify({ type: "auth", token }));
    };

    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data);
        if (msg.type === "state" && msg.data && this.mounted) {
          this.pending = msg.data as ExchangeState;
        }
      } catch {}
    };

    ws.onclose = () => {
      this.ws = null;
      if (this.mounted) {
        this.reconnectTimer = setTimeout(() => this.connect(), 1000);
      }
    };

    ws.onerror = () => {};
  }

  private flush() {
    const next = this.pending;
    if (!next || !this.mounted) return;
    this.pending = null;
    if (!this.state) {
      this.setState(next);
    } else {
      const merged = mergeState(this.state, next);
      if (merged !== this.state) {
        this.setState(merged);
      }
    }
  }

  private setState(s: ExchangeState | null) {
    this.state = s;
    for (const l of this.listeners) l();
  }

  subscribe = (listener: Listener) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };

  getSnapshot = (): ExchangeState | null => this.state;
}

// ---------------------------------------------------------------------------
// Context (just holds the store instance)
// ---------------------------------------------------------------------------

const StoreContext = createContext<ExchangeStore | null>(null);

function useStore(): ExchangeStore {
  const store = useContext(StoreContext);
  if (!store) throw new Error("ExchangeProvider missing");
  return store;
}

// ---------------------------------------------------------------------------
// Provider - creates store, manages WS lifecycle
// ---------------------------------------------------------------------------

export function ExchangeProvider({ children }: { children: ReactNode }) {
  const storeRef = useRef<ExchangeStore | null>(null);
  if (!storeRef.current) {
    storeRef.current = new ExchangeStore();
  }

  useEffect(() => {
    return storeRef.current!.start();
  }, []);

  return (
    <StoreContext.Provider value={storeRef.current}>
      {children}
    </StoreContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Selector hook - only re-renders when selected value changes
// ---------------------------------------------------------------------------

function useSelector<T>(selector: (s: ExchangeState | null) => T): T {
  const store = useStore();
  const selectorRef = useRef(selector);
  selectorRef.current = selector;

  const cachedRef = useRef<{ value: T; state: ExchangeState | null }>({
    value: selector(store.getSnapshot()),
    state: store.getSnapshot(),
  });

  const getSnapshot = useCallback(() => {
    const state = store.getSnapshot();
    if (state === cachedRef.current.state) return cachedRef.current.value;
    const next = selectorRef.current(state);
    cachedRef.current = { value: next, state };
    return next;
  }, [store]);

  return useSyncExternalStore(store.subscribe, getSnapshot, () =>
    selector(null),
  );
}

// ---------------------------------------------------------------------------
// State hooks - each only re-renders when its slice changes
// ---------------------------------------------------------------------------

export function useExchangeState(): ExchangeState | null {
  return useSelector((s) => s);
}

export function useProducts(): Product[] {
  return useSelector((s) => s?.products ?? EMPTY_PRODUCTS);
}
const EMPTY_PRODUCTS: Product[] = [];

export function useOrderBook(symbol: string): BookSnapshot | null {
  return useSelector((s) => s?.books?.[symbol] ?? null);
}

export function usePositions(): Position[] {
  return useSelector((s) => s?.positions ?? EMPTY_POSITIONS);
}
const EMPTY_POSITIONS: Position[] = [];

export function useOpenOrders(): OpenOrder[] {
  return useSelector((s) => s?.openOrders ?? EMPTY_ORDERS);
}
const EMPTY_ORDERS: OpenOrder[] = [];

export function usePnl(): number {
  return useSelector((s) => s?.pnl ?? 0);
}

export function useRecentTrades(): Trade[] {
  return useSelector((s) => s?.recentTrades ?? EMPTY_TRADES);
}
const EMPTY_TRADES: Trade[] = [];

export function useLeaderboard(): LeaderboardEntry[] {
  return useSelector((s) => s?.leaderboard ?? EMPTY_LEADERBOARD);
}
const EMPTY_LEADERBOARD: LeaderboardEntry[] = [];

export function useAllPositions(): AllPosition[] {
  return useSelector((s) => s?.allPositions ?? EMPTY_ALL_POS);
}
const EMPTY_ALL_POS: AllPosition[] = [];

export function useAllUsers(): UserEntry[] | undefined {
  return useSelector((s) => s?.allUsers);
}

export function useExchangeOpen(): boolean {
  return useSelector((s) => s?.exchangeOpen ?? false);
}

export function useAccount(): Account | null {
  return useSelector((s) => s?.account ?? null);
}

export function useConfig(): ExchangeConfig | null {
  return useSelector((s) => s?.config ?? null);
}

export function useOptionWindows(): OptionWindow[] {
  return useSelector((s) => s?.optionWindows ?? EMPTY_WINDOWS);
}
const EMPTY_WINDOWS: OptionWindow[] = [];

export function useProduct(symbol: string): Product | undefined {
  return useSelector((s) => s?.products.find((p) => p.symbol === symbol));
}

function bookMid(b: BookSnapshot | undefined): number | null {
  if (!b) return null;
  const bid = b.bids?.length ? Math.max(...b.bids.map((l) => l.price)) : null;
  const ask = b.asks?.length ? Math.min(...b.asks.map((l) => l.price)) : null;
  if (bid !== null && ask !== null) return (bid + ask) / 2;
  return b.lastTradePrice ?? null;
}

/** Sum of component mids for an ETF (null if any component has no price). */
export function useBasketValue(symbol: string): number | null {
  return useSelector((s) => {
    const p = s?.products.find((x) => x.symbol === symbol);
    const comp = p?.compositions?.[0];
    if (!s || !comp) return null;
    let total = 0;
    for (const c of comp) {
      const m = bookMid(s.books[c.symbol]);
      if (m === null) return null;
      total += c.weight * m;
    }
    return total;
  });
}

/** Wall clock in seconds, re-rendering every second (for countdowns). */
export function useNowSec(): number {
  const [now, setNow] = useState(() => Date.now() / 1000);
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now() / 1000), 1000);
    return () => clearInterval(t);
  }, []);
  return now;
}

// ---------------------------------------------------------------------------
// Generic HTTP polling
// ---------------------------------------------------------------------------

export function usePolled<T>(path: string, intervalMs = 5000): T | null {
  const [data, setData] = useState<T | null>(null);
  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const result = await apiFetch<T>(path);
        if (!cancelled) setData(result);
      } catch {}
    };
    load();
    const t = setInterval(load, intervalMs);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, [path, intervalMs]);
  return data;
}

export function useDailyResults(): Record<string, DailyResult[]> | null {
  return usePolled<Record<string, DailyResult[]>>("/api/daily-results", 10000);
}

export function useDataFiles(): DataFile[] | null {
  return usePolled<DataFile[]>("/api/data", 30000);
}

// ---------------------------------------------------------------------------
// PnL history (HTTP polling - not in WS state)
// ---------------------------------------------------------------------------

export function usePnlHistory(): unknown[] | null {
  const [data, setData] = useState<unknown[] | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchHistory() {
      const token = getSessionToken();
      if (!token) return;
      try {
        const result = await apiFetch<unknown[]>("/api/pnl-history");
        if (!cancelled) setData(result);
      } catch {}
    }

    fetchHistory();
    const interval = setInterval(fetchHistory, 5000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  return data;
}

// ---------------------------------------------------------------------------
// Price history (HTTP polling)
// ---------------------------------------------------------------------------

export type PriceHistory = Record<string, Array<{ mid: number; snapshotAt: number }>>;

export function usePriceHistory(): PriceHistory | null {
  const [data, setData] = useState<PriceHistory | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetch_() {
      try {
        const result = await apiFetch<PriceHistory>("/api/price-history");
        if (!cancelled) setData(result);
      } catch {}
    }

    fetch_();
    const interval = setInterval(fetch_, 5000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  return data;
}

// ---------------------------------------------------------------------------
// Volume matrix (HTTP polling)
// ---------------------------------------------------------------------------

export interface VolumeMatrixEntry {
  buyer: string;
  seller: string;
  volume: number;
  qty: number;
}

export function useVolumeMatrix(): VolumeMatrixEntry[] | null {
  const [data, setData] = useState<VolumeMatrixEntry[] | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetch_() {
      const token = getSessionToken();
      if (!token) return;
      try {
        const result = await apiFetch<VolumeMatrixEntry[]>("/api/volume-matrix");
        if (!cancelled) setData(result);
      } catch {}
    }

    fetch_();
    const interval = setInterval(fetch_, 5000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  return data;
}

// ---------------------------------------------------------------------------
// Mutation hooks
// ---------------------------------------------------------------------------

export function usePlaceOrder(): (
  symbol: string,
  side: string,
  price: number,
  qty: number,
  orderType: string,
) => Promise<{ orderId: string; fills: Array<{ price: number; qty: number }>; status: string }> {
  return useCallback(
    (
      symbol: string,
      side: string,
      price: number,
      qty: number,
      orderType: string,
    ) =>
      apiFetch("/api/order", {
        method: "POST",
        body: JSON.stringify({ symbol, side, price, qty, orderType }),
      }),
    [],
  );
}

export function useCancelOrder(): (orderId: string) => Promise<unknown> {
  return useCallback(
    (orderId: string) =>
      apiFetch("/api/order", {
        method: "DELETE",
        body: JSON.stringify({ orderId }),
      }),
    [],
  );
}

export function useCancelAll(): (
  symbol?: string,
  side?: string,
) => Promise<unknown> {
  return useCallback(
    (symbol?: string, side?: string) => {
      const body: Record<string, string> = {};
      if (symbol) body.symbol = symbol;
      if (side) body.side = side;
      return apiFetch("/api/cancel-all", {
        method: "POST",
        body: JSON.stringify(body),
      });
    },
    [],
  );
}

export function useEtfSwap(): (
  symbol: string,
  qty: number,
  direction: "create" | "redeem",
) => Promise<{ success: boolean; qty: number; fee: number }> {
  return useCallback(
    (symbol: string, qty: number, direction: "create" | "redeem") =>
      apiFetch("/api/etf-swap", {
        method: "POST",
        body: JSON.stringify({ symbol, qty, direction, compositionIndex: 0 }),
      }),
    [],
  );
}

/** Turns "API 400: {"error":"..."}" into "...". */
export function errorMessage(err: unknown, fallback = "Request failed"): string {
  const raw = err instanceof Error ? err.message : fallback;
  const clean = raw.replace(/^API \d+:\s*/, "").trim();
  try {
    return JSON.parse(clean).error ?? clean;
  } catch {
    return clean || fallback;
  }
}

export type ConfigUpdate = Partial<{
  etfFee: number;
  fruitMarginRate: number;
  etfMarginRate: number;
  startingCash: number;
  dayLengthMin: number;
  phase: "training" | "testing";
  simEnabled: boolean;
  enabledExpiries: number[];
  clickTrading: boolean;
}>;

export function useAdminActions() {
  return useMemo(
    () => ({
      toggleExchange: () =>
        apiFetch("/api/admin/toggle", { method: "POST" }),

      createProduct: (symbol: string, name: string, posLimit: number) =>
        apiFetch("/api/admin/product", {
          method: "POST",
          body: JSON.stringify({ action: "create", symbol, name, positionLimit: posLimit }),
        }),

      deleteProduct: (productId: string) =>
        apiFetch("/api/admin/product", {
          method: "POST",
          body: JSON.stringify({ action: "delete", productId }),
        }),

      updatePositionLimit: (productId: string, limit: number) =>
        apiFetch("/api/admin/product", {
          method: "POST",
          body: JSON.stringify({ action: "updatePositionLimit", productId, positionLimit: limit }),
        }),

      updateTickSize: (productId: string, tickSize: number) =>
        apiFetch("/api/admin/product", {
          method: "POST",
          body: JSON.stringify({ action: "updateTickSize", productId, tickSize }),
        }),

      createEtf: (
        symbol: string,
        name: string,
        posLimit: number,
        compositions: Array<Array<{ productId: string; weight: number }>>,
      ) =>
        apiFetch("/api/admin/product", {
          method: "POST",
          body: JSON.stringify({
            action: "createEtf",
            symbol,
            name,
            positionLimit: posLimit,
            compositions,
          }),
        }),

      resetAll: () =>
        apiFetch("/api/admin/reset", {
          method: "POST",
          body: JSON.stringify({}),
        }),

      resetUser: (targetUserId: string) =>
        apiFetch("/api/admin/reset", {
          method: "POST",
          body: JSON.stringify({ targetUserId }),
        }),

      getAllUsers: () => apiFetch<UserEntry[]>("/api/admin/users"),

      setNoPositionLimit: (targetUserId: string, value: boolean) =>
        apiFetch("/api/admin/no-pos-limit", {
          method: "POST",
          body: JSON.stringify({ targetUserId, value }),
        }),

      setRateLimit: (targetUserId: string, rateLimitMs: number) =>
        apiFetch("/api/admin/rate-limit", {
          method: "POST",
          body: JSON.stringify({ targetUserId, rateLimitMs }),
        }),

      updateConfig: (update: ConfigUpdate) =>
        apiFetch<ExchangeConfig>("/api/admin/config", {
          method: "POST",
          body: JSON.stringify(update),
        }),

      endDay: () => apiFetch("/api/admin/end-day", { method: "POST" }),
    }),
    [],
  );
}
