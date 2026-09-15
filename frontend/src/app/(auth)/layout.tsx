import { Shield } from "lucide-react";
import { Suspense } from "react";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <main
      id="main-content"
      className="flex min-h-screen items-center justify-center bg-gradient-to-br from-background via-muted/50 to-background p-4"
    >
      <div className="w-full max-w-md">
        {/* Logo */}
        <div className="mb-8 flex flex-col items-center gap-2">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
            <Shield aria-hidden="true" className="h-7 w-7" />
          </div>
          <p className="text-2xl font-bold tracking-tight">ComplianceForge</p>
          <p className="text-sm text-muted-foreground">
            Enterprise GRC Platform
          </p>
        </div>

        {/* Card container */}
        <div className="rounded-xl border bg-card p-6 shadow-sm">
          <Suspense
            fallback={
              <div
                aria-busy="true"
                role="status"
                aria-label="Loading account workflow"
                className="h-48 animate-pulse rounded-lg bg-muted motion-reduce:animate-none"
              />
            }
          >
            {children}
          </Suspense>
        </div>
      </div>
    </main>
  );
}
