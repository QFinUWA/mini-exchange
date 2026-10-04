import type { Metadata } from "next";
import "./globals.css";
import { ExchangeProvider } from "@/lib/exchange";
import { AuthProvider } from "@/lib/auth";

export const metadata: Metadata = {
  title: "Mini Exchange",
  description: "QFin Trading Exchange",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="dark">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet" />
      </head>
      <body className="min-h-screen bg-zinc-950 text-zinc-100 antialiased">
        <AuthProvider>
          <ExchangeProvider>{children}</ExchangeProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
