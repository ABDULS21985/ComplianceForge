import { Card, CardContent } from '@/components/ui/card';

function FrameworkCardSkeleton() {
  return (
    <Card>
      <div className="h-1.5 animate-pulse rounded-t-lg bg-muted" />
      <CardContent className="p-6">
        <div className="animate-pulse space-y-3">
          <div className="h-5 w-3/4 rounded bg-muted" />
          <div className="h-4 w-1/2 rounded bg-muted" />
          <div className="flex items-center gap-2">
            <div className="h-5 w-16 rounded-full bg-muted" />
            <div className="h-5 w-20 rounded-full bg-muted" />
          </div>
          <div className="h-4 w-1/3 rounded bg-muted" />
        </div>
      </CardContent>
    </Card>
  );
}

export default function FrameworksLoading() {
  return (
    <div className="space-y-6">
      {/* Header skeleton */}
      <div className="space-y-2">
        <div className="h-9 w-48 animate-pulse rounded bg-muted" />
        <div className="h-5 w-96 animate-pulse rounded bg-muted" />
      </div>

      {/* Grid skeleton */}
      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 9 }).map((_, i) => (
          <FrameworkCardSkeleton key={i} />
        ))}
      </div>
    </div>
  );
}
