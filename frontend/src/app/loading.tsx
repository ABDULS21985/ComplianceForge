import { ResourceState } from '@/components/data/resource-state';

export default function ApplicationLoading() {
  return (
    <main id="main-content" className="min-h-screen p-6">
      <h1 className="sr-only">Loading ComplianceForge</h1>
      <ResourceState
        kind="loading"
        loadingLayout="detail"
        title="Loading ComplianceForge"
      />
    </main>
  );
}
