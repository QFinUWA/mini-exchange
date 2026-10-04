"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { apiFetch } from "@/lib/exchange";
import { useAuth } from "@/lib/auth";

type Mode = "login" | "register";

export default function LoginPage() {
  const router = useRouter();
  const { sessionToken, login } = useAuth();

  const [mode, setMode] = useState<Mode>("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (sessionToken) router.replace("/dashboard");
  }, [sessionToken, router]);

  const submit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const result = await apiFetch<{
        token: string;
        username: string;
        isAdmin: boolean;
      }>("/api/auth", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      });
      login(result.token, result.username, result.isAdmin);
      router.replace("/dashboard");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Something went wrong";
      setError(message.replace(/^.*?Error:\s*/, ""));
    } finally {
      setSubmitting(false);
    }
  };

  const switchMode = (next: Mode) => {
    setMode(next);
    setError(null);
  };

  return (
    <div className="flex items-center justify-center min-h-screen px-4 bg-gradient-to-b from-[#0a0a1a] via-[#0a0a0f] to-[#0a0a0f]">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-white">Mini Exchange</h1>
          <p className="mt-1 text-sm text-zinc-500">QFin Trading Competition</p>
        </div>

        <div className="bg-zinc-900/80 backdrop-blur rounded-2xl border border-zinc-800/60 p-6 shadow-2xl shadow-black/20">
          <div className="flex gap-1 mb-5 p-1 bg-zinc-950 rounded-lg">
            <button
              type="button"
              onClick={() => switchMode("login")}
              className={`flex-1 py-2 text-sm font-medium rounded-md transition-all ${
                mode === "login"
                  ? "bg-zinc-800 text-white shadow-sm"
                  : "text-zinc-500 hover:text-zinc-300"
              }`}
            >
              Sign In
            </button>
            <button
              type="button"
              onClick={() => switchMode("register")}
              className={`flex-1 py-2 text-sm font-medium rounded-md transition-all ${
                mode === "register"
                  ? "bg-zinc-800 text-white shadow-sm"
                  : "text-zinc-500 hover:text-zinc-300"
              }`}
            >
              Register
            </button>
          </div>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
            className="space-y-4"
          >
            <div>
              <label htmlFor="username" className="block text-sm font-medium text-zinc-400 mb-1.5">
                Team Name
              </label>
              <input
                id="username"
                type="text"
                required
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="Enter your team name"
                className="w-full px-3.5 py-2.5 bg-zinc-950 border border-zinc-800 text-white rounded-lg placeholder-zinc-600 focus:outline-none focus:ring-2 focus:ring-blue-500/40 focus:border-blue-500/40 transition-all"
              />
            </div>

            <div>
              <label htmlFor="password" className="block text-sm font-medium text-zinc-400 mb-1.5">
                Password
              </label>
              <input
                id="password"
                type="password"
                required
                autoComplete={mode === "login" ? "current-password" : "new-password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter password"
                className="w-full px-3.5 py-2.5 bg-zinc-950 border border-zinc-800 text-white rounded-lg placeholder-zinc-600 focus:outline-none focus:ring-2 focus:ring-blue-500/40 focus:border-blue-500/40 transition-all"
              />
            </div>

            {error && (
              <div className="px-3.5 py-2.5 text-sm text-red-300 bg-red-500/10 border border-red-500/20 rounded-lg">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={submitting}
              className="w-full py-2.5 bg-gradient-to-r from-blue-600 to-blue-500 hover:from-blue-500 hover:to-blue-400 disabled:from-blue-800 disabled:to-blue-800 disabled:cursor-not-allowed text-white font-semibold rounded-lg transition-all shadow-lg shadow-blue-500/20"
            >
              {submitting
                ? mode === "login"
                  ? "Signing in..."
                  : "Creating team..."
                : mode === "login"
                  ? "Sign In"
                  : "Create Team"}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
