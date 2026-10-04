"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter, usePathname } from "next/navigation";
import { useProducts } from "@/lib/exchange";
import { useAuth } from "@/lib/auth";
import { OrderBook } from "@/components/OrderBook";
import { PositionsSidebar } from "@/components/PositionsSidebar";
import { OpenOrders } from "@/components/OpenOrders";
import { TradeHistory } from "@/components/TradeHistory";
import { AccountPanel } from "@/components/AccountPanel";

export default function DashboardPage() {
  const router = useRouter();
  const pathname = usePathname();
  const { sessionToken, username, isAdmin, logout } = useAuth();
  const products = useProducts();

  useEffect(() => {
    if (sessionToken === null) {
      const stored = typeof window !== "undefined" ? localStorage.getItem("exchange_auth") : null;
      if (!stored) router.replace("/login");
    }
  }, [sessionToken, router]);

  if (!sessionToken) {
    return (
      <div className="min-h-screen flex items-center justify-center text-zinc-500 text-sm">
        Loading...
      </div>
    );
  }

  const navLink = (href: string, label: string) => {
    const active = pathname === href;
    return (
      <Link
        href={href}
        className={`px-3 py-1.5 text-sm rounded-lg transition-all ${
          active
            ? "bg-zinc-800 text-white font-medium"
            : "text-zinc-400 hover:text-white hover:bg-zinc-800/50"
        }`}
      >
        {label}
      </Link>
    );
  };

  return (
    <div className="h-screen flex flex-col overflow-hidden">
      <header className="flex items-center justify-between px-4 py-2.5 border-b border-zinc-800/60 bg-[#0a0a0f]/90 backdrop-blur-xl">
        <div className="flex items-center gap-5">
          <Link href="/dashboard" className="text-base font-semibold text-white">
            Mini Exchange
          </Link>
          <nav className="flex items-center gap-1">
            {navLink("/dashboard", "Trade")}
            {navLink("/leaderboard", "Leaderboard")}
            {navLink("/positions", "Positions")}
            {navLink("/prices", "Prices")}
            {navLink("/data", "Data")}
            {isAdmin && navLink("/admin", "Admin")}
          </nav>
          <span className="text-xs text-zinc-600">
            {products.length} market{products.length !== 1 ? "s" : ""}
          </span>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-full bg-zinc-800 flex items-center justify-center text-xs font-bold text-zinc-300 uppercase">
              {username?.[0] ?? "?"}
            </div>
            <span className="text-sm font-medium text-zinc-300">{username}</span>
            {isAdmin && (
              <span className="px-1.5 py-0.5 text-[10px] font-semibold rounded-md bg-violet-500/15 text-violet-400 border border-violet-500/20">
                Admin
              </span>
            )}
          </div>
          <button
            onClick={() => {
              logout();
              router.replace("/login");
            }}
            className="px-3 py-1.5 text-sm rounded-lg text-zinc-400 hover:text-white hover:bg-zinc-800/60 transition-all"
          >
            Sign out
          </button>
        </div>
      </header>

      <div className="flex-1 flex min-h-0">
        <main className="flex-1 overflow-x-auto overflow-y-hidden">
          <div className="flex h-full p-2 gap-2">
            {products.length === 0 ? (
              <div className="flex items-center justify-center w-full text-zinc-500 text-sm">
                No active markets
              </div>
            ) : (
              products.map((p) => (
                <OrderBook
                  key={p.id}
                  symbol={p.symbol}
                  compositions={p.compositions}
                />
              ))
            )}
          </div>
        </main>

        <aside className="w-[360px] shrink-0 border-l border-zinc-800/60 bg-zinc-950/50 flex flex-col min-h-0">
          <div className="shrink-0">
            <AccountPanel />
          </div>
          <div className="shrink-0">
            <PositionsSidebar />
          </div>
          <div className="flex-1 min-h-0 flex flex-col">
            <OpenOrders />
          </div>
          <div className="flex-1 min-h-0 flex flex-col">
            <TradeHistory />
          </div>
        </aside>
      </div>
    </div>
  );
}
