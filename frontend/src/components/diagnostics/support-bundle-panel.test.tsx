import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SUPPORT_TEST_TENANT, supportBundleFixture } from '@/test/support-bundle-fixture';
import { axe } from 'jest-axe';
import { SESSION_EXPIRED_EVENT } from '@/lib/auth-constants';
import { SupportBundlePanel } from './support-bundle-panel';
import { useAuthStore } from '@/store/auth-store';
import type { User } from '@/types';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({ generate: vi.fn(), verify: vi.fn(), logout: vi.fn() }));
vi.mock('@/lib/api', () => ({ default: { diagnostics: { generateSupportBundle: mocks.generate }, auth: { logout: mocks.logout } } }));
vi.mock('@/lib/support-bundle', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/support-bundle')>(), verifySupportBundle: mocks.verify,
}));

const user: User = {
  id: 'bba53cd8-dabc-4c88-97bc-319aa1bc79ed', organization_id: SUPPORT_TEST_TENANT,
  email: 'admin@example.test', first_name: 'Tenant', last_name: 'Administrator',
  status: 'active', is_super_admin: false, language: 'en',
  created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
};
const OriginalURL = globalThis.URL;
let createUrl: (blob: Blob | MediaSource) => string;
let revokeUrl: (url: string) => void;

function panel(permissions = ['read', 'configure']) {
  return <SupportBundlePanel organizationId={SUPPORT_TEST_TENANT} permissions={{ settings: permissions }} permissionsVerified />;
}
function preview() { fireEvent.click(screen.getByRole('button', { name: 'Preview support bundle' })); }
function generate() {
  fireEvent.click(screen.getByRole('checkbox', { name: /I consent to generate/i }));
  fireEvent.click(screen.getByRole('button', { name: 'Generate bundle' }));
}
async function ready() {
  preview(); generate();
  await screen.findByRole('heading', { name: 'Bundle integrity checked' });
}

describe('support bundle preview, consent and local-only lifecycle', () => {
  beforeEach(() => {
    useAuthStore.setState({ user, isAuthenticated: true, isLoggingOut: false });
    const fixture = supportBundleFixture();
    mocks.generate.mockReset().mockResolvedValue(fixture.attachment);
    mocks.logout.mockReset().mockReturnValue(new Promise(() => undefined));
    mocks.verify.mockReset().mockResolvedValue({ blob: fixture.attachment.blob, filename: fixture.headers['content-disposition'].split('filename=')[1], sha256: fixture.attachment.supportBundleSha256, manifest: fixture.manifest });
    createUrl = vi.fn<(blob: Blob | MediaSource) => string>().mockReturnValue('blob:http://localhost/support-local');
    revokeUrl = vi.fn<(url: string) => void>();
    vi.stubGlobal('URL', class extends OriginalURL {
      static createObjectURL = createUrl;
      static revokeObjectURL = revokeUrl;
    });
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
  });
  afterEach(() => {
    cleanup(); useAuthStore.setState({ user: null, isAuthenticated: false, isLoggingOut: false });
    vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers();
  });

  it('fails closed for read-only, unverified, unauthenticated and mismatched-tenant access', () => {
    const view = render(panel(['read']));
    expect(screen.queryByRole('button', { name: 'Preview support bundle' })).not.toBeInTheDocument();
    view.rerender(<SupportBundlePanel organizationId={SUPPORT_TEST_TENANT} permissions={{ settings: ['configure'] }} permissionsVerified={false} />);
    expect(screen.queryByRole('button', { name: 'Preview support bundle' })).not.toBeInTheDocument();
    act(() => useAuthStore.getState().clearAuth());
    view.rerender(panel());
    expect(screen.queryByRole('button', { name: 'Preview support bundle' })).not.toBeInTheDocument();
    act(() => useAuthStore.setState({ user: { ...user, organization_id: 'a23fec3b-3544-49f3-99eb-af26b93eec07' }, isAuthenticated: true }));
    expect(screen.queryByRole('button', { name: 'Preview support bundle' })).not.toBeInTheDocument();
    expect(mocks.generate).not.toHaveBeenCalled();
  });

  it('previews exact scope/exclusions without generation, supports keyboard focus/Escape, and passes axe', async () => {
    const keyboard = userEvent.setup(); render(panel()); preview();
    expect(screen.getByRole('heading', { name: 'Included operational metadata' })).toHaveFocus();
    expect(screen.getByText(/Tenant UUID included:/).closest('p')).toHaveTextContent(SUPPORT_TEST_TENANT);
    expect(screen.getByText(/Scope: health_and_posture/)).toBeInTheDocument();
    expect(screen.getByRole('table', { name: /Exactly four fixed/i })).toBeInTheDocument();
    expect(screen.getByText('User identifiers')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Generate bundle' })).toBeDisabled();
    expect(mocks.generate).not.toHaveBeenCalled();
    await keyboard.tab();
    expect(screen.getByRole('checkbox', { name: /I consent/i })).toHaveFocus();
    expect((await axe(screen.getByRole('dialog'))).violations).toEqual([]);
    await keyboard.keyboard('{Escape}');
    expect(screen.getByRole('button', { name: 'Preview support bundle' })).toHaveFocus();
  });

  it('consumes consent and focuses a safe error; retry requires an explicit new consent', async () => {
    mocks.generate.mockRejectedValue({ status: 503, message: 'secret=hidden; host=private.internal' });
    render(panel()); preview(); generate();
    const error = await screen.findByRole('alert');
    expect(error).toHaveFocus();
    expect(error).toHaveTextContent('No automatic retry occurred');
    expect(error).not.toHaveTextContent(/secret|private.internal/);
    expect(screen.getByRole('checkbox', { name: /I consent/i })).not.toBeChecked();
    fireEvent.click(screen.getByRole('button', { name: 'Generate bundle' }));
    expect(mocks.generate).toHaveBeenCalledOnce();
    generate(); await waitFor(() => expect(mocks.generate).toHaveBeenCalledTimes(2));
  });

  it('cancels an ambiguous in-flight generation and discards its late completion', async () => {
    let resolve: (value: unknown) => void = () => undefined;
    mocks.generate.mockImplementation(() => new Promise((done) => { resolve = done; }));
    render(panel()); preview(); generate();
    const signal = mocks.generate.mock.calls[0][1] as AbortSignal;
    fireEvent.click(screen.getByRole('button', { name: 'Cancel generation' }));
    expect(signal.aborted).toBe(true);
    expect(screen.getByText(/server may already have recorded/i)).toBeInTheDocument();
    await act(async () => resolve(supportBundleFixture().attachment));
    expect(mocks.verify).not.toHaveBeenCalled();
    expect(screen.queryByRole('heading', { name: 'Bundle integrity checked' })).not.toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /I consent/i })).not.toBeChecked();
  });

  it('downloads only a verified Blob with a canonical filename and promptly revokes its URL', async () => {
    render(panel()); await ready();
    expect(screen.getByRole('heading', { name: 'Bundle integrity checked' })).toHaveFocus();
    expect(screen.getByText(/not a signature, proof of origin, or encryption/i)).toBeInTheDocument();
    vi.useFakeTimers(); fireEvent.click(screen.getByRole('button', { name: 'Download local ZIP' }));
    expect(createUrl).toHaveBeenCalledOnce();
    const link = vi.mocked(HTMLAnchorElement.prototype.click).mock.contexts[0] as HTMLAnchorElement;
    expect(link.href).toBe('blob:http://localhost/support-local');
    expect(link.download).toMatch(/^complianceforge-support-[0-9a-f-]+\.zip$/);
    expect(document.querySelector('a[download]')).toBeNull();
    expect(screen.getByText(/successful receipt has not been confirmed/i)).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(revokeUrl).toHaveBeenCalledWith('blob:http://localhost/support-local');
  });

  it.each(['permission', 'session', 'logout intent', 'expiry event', 'offline', 'unmount'])('clears prepared data/consent and revokes URLs on %s', async (condition) => {
    const view = render(panel()); await ready();
    fireEvent.click(screen.getByRole('button', { name: 'Download local ZIP' }));
    if (condition === 'permission') view.rerender(panel(['read']));
    if (condition === 'session') act(() => useAuthStore.getState().clearAuth());
    if (condition === 'logout intent') act(() => { void useAuthStore.getState().logout(); });
    if (condition === 'expiry event') act(() => window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT)));
    if (condition === 'offline') {
      vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
      act(() => window.dispatchEvent(new Event('offline')));
      expect(screen.queryByRole('checkbox', { name: /I consent/i })).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Preview support bundle' })).toBeDisabled();
    }
    if (condition === 'unmount') view.unmount();
    expect(revokeUrl).toHaveBeenCalledWith('blob:http://localhost/support-local');
    expect(screen.queryByRole('heading', { name: 'Bundle integrity checked' })).not.toBeInTheDocument();
  });

  it('starts another generation with cleared consent and restores preview focus', async () => {
    render(panel()); await ready();
    fireEvent.click(screen.getByRole('button', { name: 'Discard and prepare another' }));
    expect(screen.getByRole('heading', { name: 'Included operational metadata' })).toHaveFocus();
    expect(screen.getByRole('checkbox', { name: /I consent/i })).not.toBeChecked();
    expect(screen.getByRole('button', { name: 'Generate bundle' })).toBeDisabled();
  });
});
