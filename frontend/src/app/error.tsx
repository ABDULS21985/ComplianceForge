'use client';

import { ResourceState } from '@/components/data/resource-state';

export default function ApplicationError({ reset }: { reset: () => void }) {
  return (
    <main id="main-content" className="flex min-h-screen items-center p-6">
      <ResourceState
        headingLevel={1}
        kind="error"
        title="This page could not be loaded"
        description="The application encountered an unexpected problem. Retry without exposing diagnostic details."
        onRetry={reset}
      />
    </main>
  );
}
