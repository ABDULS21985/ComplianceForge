import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { ResourceState } from '@/components/data/resource-state';

export default function NotFoundPage() {
  return (
    <main id="main-content" className="flex min-h-screen items-center p-6">
      <ResourceState
        headingLevel={1}
        kind="empty"
        title="Page not found"
        description="The requested page does not exist or is no longer available."
        action={
          <Button asChild>
            <Link href="/dashboard">Return to dashboard</Link>
          </Button>
        }
      />
    </main>
  );
}
