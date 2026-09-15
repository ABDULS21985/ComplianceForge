"use client";

import "./globals.css";

import { OfflineBanner, RouteFocusManager } from "@/components/layout/app-status";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Inter } from "next/font/google";
import { ProductAccessFeedback } from "@/components/layout/product-access-feedback";
import { purgeLegacyBrowserCredentials } from "@/lib/auth";
import { QueryStatus } from "@/components/data/query-status";
import { ThemeProvider } from "next-themes";
import { Toaster } from "sonner";

const inter = Inter({ subsets: ["latin"], variable: "--font-sans" });

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: 1,
      },
    },
  });
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [queryClient] = useState(() => makeQueryClient());

  useEffect(() => {
    purgeLegacyBrowserCredentials();
  }, []);

  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <title>ComplianceForge</title>
        <meta name="description" content="Enterprise GRC Platform" />
      </head>
      <body className={`${inter.variable} font-sans`}>
        <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
          <QueryClientProvider client={queryClient}>
            <RouteFocusManager />
            <OfflineBanner />
            <QueryStatus />
            {children}
            <ProductAccessFeedback />
            <Toaster richColors position="top-right" />
          </QueryClientProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
