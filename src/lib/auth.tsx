"use client";

import { createContext, useContext, useState, useEffect, ReactNode, useCallback } from "react";

interface AuthState {
  sessionToken: string | null;
  username: string | null;
  isAdmin: boolean;
}

interface AuthContextType extends AuthState {
  login: (token: string, username: string, isAdmin: boolean) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [auth, setAuth] = useState<AuthState>({
    sessionToken: null,
    username: null,
    isAdmin: false,
  });

  useEffect(() => {
    const stored = localStorage.getItem("exchange_auth");
    if (stored) {
      try {
        setAuth(JSON.parse(stored));
      } catch {}
    }
  }, []);

  const login = useCallback((token: string, username: string, isAdmin: boolean) => {
    const state = { sessionToken: token, username, isAdmin };
    setAuth(state);
    localStorage.setItem("exchange_auth", JSON.stringify(state));
  }, []);

  const logout = useCallback(() => {
    setAuth({ sessionToken: null, username: null, isAdmin: false });
    localStorage.removeItem("exchange_auth");
  }, []);

  return (
    <AuthContext.Provider value={{ ...auth, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
