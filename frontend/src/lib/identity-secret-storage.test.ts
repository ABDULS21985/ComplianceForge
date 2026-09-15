import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const identityBrowserSources = [
  '../app/(auth)/accept-invitation/page.tsx',
  '../app/(auth)/login/page.tsx',
  '../app/(auth)/reset-password/page.tsx',
  '../app/(auth)/verify-email/page.tsx',
  '../components/identity/mfa-panel.tsx',
  '../components/identity/passkeys-panel.tsx',
  '../components/identity/recovery-codes-dialog.tsx',
  '../components/identity/step-up-dialog.tsx',
  '../components/identity/use-ephemeral-token.ts',
  './identity.ts',
] as const;

describe('identity browser secret boundary', () => {
  it.each(identityBrowserSources)('never persists credentials from %s', (relativePath) => {
    const source = readFileSync(new URL(relativePath, import.meta.url), 'utf8');

    expect(source).not.toMatch(/\b(?:indexedDB|localStorage|sessionStorage)\b|document\.cookie/);
  });
});
