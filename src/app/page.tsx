"use client";
import { useAuth } from "@/lib/auth";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function Home() {
  const { sessionToken } = useAuth();
  const router = useRouter();

  useEffect(() => {
    router.replace(sessionToken ? "/dashboard" : "/login");
  }, [sessionToken, router]);

  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="text-zinc-500">Loading...</div>
    </div>
  );
}
