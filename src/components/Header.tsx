"use client";

import Link from "next/link";
import { useRouter, usePathname } from "next/navigation";
import { useAuth } from "@/lib/auth";

export default function Header() {
  const { username, isAdmin, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  const handleLogout = () => {
    logout();
    router.replace("/login");
  };

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
    <header className="sticky top-0 z-40 bg-[#0a0a0f]/90 backdrop-blur-xl border-b border-zinc-800/60">
      <div className="h-14 px-5 flex items-center justify-between">
        <div className="flex items-center gap-6">
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
        </div>
        <div className="flex items-center gap-3">
          {username && (
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 rounded-full bg-zinc-800 flex items-center justify-center text-xs font-bold text-zinc-300 uppercase">
                {username[0]}
              </div>
              <span className="text-sm font-medium text-zinc-300">{username}</span>
              {isAdmin && (
                <span className="px-1.5 py-0.5 text-[10px] font-semibold rounded-md bg-violet-500/15 text-violet-400 border border-violet-500/20">
                  Admin
                </span>
              )}
            </div>
          )}
          <button
            type="button"
            onClick={handleLogout}
            className="px-3 py-1.5 text-sm rounded-lg text-zinc-400 hover:text-white hover:bg-zinc-800/60 transition-all"
          >
            Sign out
          </button>
        </div>
      </div>
    </header>
  );
}
