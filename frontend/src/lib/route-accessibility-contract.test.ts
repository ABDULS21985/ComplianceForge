// @vitest-environment node

import { describe, expect, it } from 'vitest';
import { join, relative } from 'node:path';
import { readdirSync, readFileSync } from 'node:fs';

const APP_ROOT = join(process.cwd(), 'src/app');

function findPages(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return findPages(path);
    return entry.name === 'page.tsx' ? [path] : [];
  });
}

function source(path: string) {
  return readFileSync(path, 'utf8');
}

const directRequestRoutes = [
  '(dashboard)/activity/page.tsx',
  '(dashboard)/knowledge/page.tsx',
  '(dashboard)/search/page.tsx',
  '(dashboard)/settings/branding/page.tsx',
  'board-portal/page.tsx',
  'vendor-portal/page.tsx',
];

describe('routed-page accessibility contract', () => {
  it('inventories every current page and gives each rendered route an h1', () => {
    const pages = findPages(APP_ROOT);
    expect(pages).toHaveLength(62);

    for (const page of pages) {
      const contents = source(page);
      if (contents.includes('redirect(')) continue;
      expect(contents, relative(APP_ROOT, page)).toMatch(/<h1\b/);
    }
  });

  it('mounts route focus, connectivity, query failure, and segment fallback coverage at the root', () => {
    const layout = source(join(APP_ROOT, 'layout.tsx'));
    expect(layout).toContain('<RouteFocusManager />');
    expect(layout).toContain('<OfflineBanner />');
    expect(layout).toContain('<QueryStatus />');

    expect(source(join(APP_ROOT, 'loading.tsx'))).toContain('kind="loading"');
    expect(source(join(APP_ROOT, 'error.tsx'))).toContain('kind="error"');
    expect(source(join(APP_ROOT, 'not-found.tsx'))).toContain('kind="empty"');
  });

  it.each(directRequestRoutes)(
    '%s has explicit semantic loading and safe failure recovery',
    (route) => {
      const contents = source(join(APP_ROOT, route));
      expect(contents).toContain('ResourceState');
      expect(contents).toMatch(/kind="loading"/);
      expect(contents).toMatch(/kind="error"/);
      expect(contents).toMatch(/onRetry=/);
    },
  );

  it('keeps landmarks in authenticated, auth, onboarding, and token portal shells', () => {
    expect(source(join(APP_ROOT, '(dashboard)/layout.tsx'))).toMatch(
      /<main\s+[\s\S]*?id="main-content"/,
    );
    expect(source(join(APP_ROOT, '(auth)/layout.tsx'))).toMatch(
      /<main\s+[\s\S]*?id="main-content"/,
    );
    expect(source(join(APP_ROOT, 'onboard/page.tsx'))).toContain(
      '<main id="main-content"',
    );
    for (const portal of ['board-portal/page.tsx', 'vendor-portal/page.tsx']) {
      expect(source(join(APP_ROOT, portal))).toContain('<main id="main-content"');
    }
  });
});
