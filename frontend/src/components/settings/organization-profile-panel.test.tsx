import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ORGANIZATION_TEST_ACTOR, ORGANIZATION_TEST_TENANT, organizationProfileFixture } from '@/test/organization-profile-fixture';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { axe } from 'jest-axe';
import { organizationProfileKeys } from '@/lib/organization-profile-hooks';
import { OrganizationProfilePanel } from './organization-profile-panel';
import { SESSION_EXPIRED_EVENT } from '@/lib/auth-constants';
import { useAuthStore } from '@/store/auth-store';
import type { User } from '@/types';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({ permissions: vi.fn(), get: vi.fn(), update: vi.fn(), logout: vi.fn() }));
vi.mock('@/lib/api', () => ({ default: { access: { myPermissions: mocks.permissions }, settings: { getOrg: mocks.get, updateOrg: mocks.update }, auth: { logout: mocks.logout } } }));
const user: User = { id: ORGANIZATION_TEST_ACTOR, organization_id: ORGANIZATION_TEST_TENANT, first_name: 'Admin', last_name: 'Reviewer', email: 'admin@example.test', status: 'active', is_super_admin: false, language: 'en', created_at: '2026-09-15T07:00:00Z', updated_at: '2026-09-15T07:00:00Z' };
let client: QueryClient;
function panel() { return <QueryClientProvider client={client}><OrganizationProfilePanel /></QueryClientProvider>; }
async function loaded() { await waitFor(() => expect(screen.getByRole('button', { name: 'Edit organisation profile' })).toBeEnabled()); }
async function edit() { await loaded(); fireEvent.click(screen.getByRole('button', { name: 'Edit organisation profile' })); }
function changeAndReview() {
  fireEvent.change(screen.getByRole('textbox', { name: 'Organisation name' }), { target: { value: 'Changed Enterprise' } });
  fireEvent.change(screen.getByRole('textbox', { name: 'Change reason' }), { target: { value: 'Reviewed legal profile update' } });
  fireEvent.click(screen.getByRole('button', { name: 'Review changes' }));
}

describe('enterprise organisation profile settings workflow', () => {
  beforeEach(() => {
    client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { gcTime: 0 } } });
    useAuthStore.setState({ user, isAuthenticated: true, isLoggingOut: false });
    mocks.permissions.mockReset().mockResolvedValue({ data: { settings: ['read', 'configure'] } });
    mocks.get.mockReset().mockResolvedValue(organizationProfileFixture());
    mocks.update.mockReset().mockResolvedValue(organizationProfileFixture({ name: 'Changed Enterprise', version: 5 }));
    mocks.logout.mockReset().mockReturnValue(new Promise(() => undefined));
  });
  afterEach(() => { cleanup(); client.clear(); useAuthStore.setState({ user: null, isAuthenticated: false, isLoggingOut: false }); vi.restoreAllMocks(); });

  it('fails closed without verified settings read permission, including role-name fallback', async () => {
    mocks.permissions.mockResolvedValue({ data: {} });
    useAuthStore.setState({ user: { ...user, roles: [{ id: user.id, slug: 'org_admin', name: 'Administrator', is_custom: false, is_system_role: true }] } });
    render(panel()); await screen.findByRole('heading', { name: 'Organisation profile access unavailable' }); expect(mocks.get).not.toHaveBeenCalled();
  });
  it('shows only returned read-only projection fields, never fills hidden fields or trusts missing metadata', async () => {
    mocks.get.mockResolvedValue({ data: { name: 'Authorised masked projection', tier: '***' } });
    render(panel()); await screen.findByText('Authorised masked projection');
    expect(screen.getByRole('button', { name: 'Edit organisation profile' })).toBeDisabled();
    expect(screen.queryByText('Legal name')).not.toBeInTheDocument(); expect(screen.queryByText('Status (read-only)')).not.toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument(); expect(screen.getByText(/missing fields are not assumed blank/i)).toBeInTheDocument();
  });
  it('treats editable metadata as projection evidence, not a configure grant', async () => {
    mocks.permissions.mockResolvedValue({ data: { settings: ['read'] } }); render(panel()); await screen.findByText('Example Enterprise');
    expect(screen.queryByRole('button', { name: 'Edit organisation profile' })).not.toBeInTheDocument(); expect(mocks.update).not.toHaveBeenCalled();
  });
  it('validates linked fields and mandatory reason before opening confirmation', async () => {
    render(panel()); await edit(); expect(screen.getByRole('textbox', { name: 'Organisation name' })).toHaveFocus();
    fireEvent.click(screen.getByRole('button', { name: 'Review changes' })); expect(screen.getByRole('alert')).toHaveFocus();
    expect(screen.getByRole('textbox', { name: 'Change reason' })).toHaveAttribute('aria-invalid', 'true');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(); expect(mocks.update).not.toHaveBeenCalled();
  });
  it('opens confirmation and saves only reviewed changes without mutation-cache drafts', async () => {
    render(panel()); await edit();
    changeAndReview(); expect(screen.getByRole('alertdialog', { name: 'Confirm organisation profile changes' })).toBeInTheDocument(); expect(mocks.update).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Confirm and save profile' }));
    await screen.findByText(/Organisation profile saved\./);
    expect(mocks.update).toHaveBeenCalledWith({ expected_version: 4, reason: 'Reviewed legal profile update', name: 'Changed Enterprise' }, expect.any(AbortSignal));
    expect(client.getMutationCache().getAll()).toHaveLength(0); expect(screen.queryByRole('textbox', { name: 'Change reason' })).not.toBeInTheDocument();
  });
  it('asks before unsaved dismissal and reopens with no discarded fields or reason', async () => {
    const keyboard = userEvent.setup(); render(panel()); await edit();
    fireEvent.change(screen.getByRole('textbox', { name: 'Legal name' }), { target: { value: 'Discard me' } });
    await keyboard.keyboard('{Escape}'); expect(screen.getByRole('alertdialog', { name: 'Discard local organisation draft?' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' })); expect(screen.getByRole('textbox', { name: 'Legal name' })).toHaveValue('Discard me');
    fireEvent.click(screen.getByRole('button', { name: 'Cancel edit' })); fireEvent.click(screen.getByRole('button', { name: 'Discard draft' }));
    await edit(); expect(screen.getByRole('textbox', { name: 'Legal name' })).toHaveValue('Example Enterprise Ltd'); expect(screen.getByRole('textbox', { name: 'Change reason' })).toHaveValue(''); expect(mocks.update).not.toHaveBeenCalled();
  });
  it('discards conflicted draft and locks editing until explicit successful reload/review', async () => {
    mocks.update.mockRejectedValue({ status: 409, message: 'database=private' }); render(panel()); await edit(); changeAndReview();
    fireEvent.click(screen.getByRole('button', { name: 'Confirm and save profile' }));
    await screen.findByText(/Another administrator changed this profile/); expect(screen.getByRole('alert')).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Edit organisation profile' })).toBeDisabled(); expect(screen.queryByRole('textbox')).not.toBeInTheDocument(); expect(mocks.update).toHaveBeenCalledOnce();
    mocks.get.mockResolvedValue(organizationProfileFixture({ name: 'Concurrent updated enterprise', version: 5 })); fireEvent.click(screen.getByRole('button', { name: 'Reload profile' }));
    await edit(); expect(screen.getByRole('textbox', { name: 'Organisation name' })).toHaveValue('Concurrent updated enterprise'); expect(screen.getByText(/Editing version 5/)).toBeInTheDocument();
  });
  it('cannot leak a mismatched tenant profile even when metadata says editable', async () => {
    mocks.get.mockResolvedValue(organizationProfileFixture({ id: 'c1a6814f-d890-47b6-a3e3-39c1b9aff60f' })); render(panel());
    await screen.findByRole('heading', { name: 'Organisation profile scope unavailable' }); expect(screen.queryByText('Example Enterprise')).not.toBeInTheDocument();
  });
  it.each(['read loss', 'configure loss', 'logout intent', 'tenant change', 'offline', 'session expiry', 'unmount'])('aborts a pending mutation and discards late completion on %s', async (condition) => {
    let resolve: (value: unknown) => void = () => undefined;
    mocks.update.mockImplementation(() => new Promise((done) => { resolve = done; }));
    const view = render(panel()); await edit(); changeAndReview(); fireEvent.click(screen.getByRole('button', { name: 'Confirm and save profile' }));
    const signal = mocks.update.mock.calls[0][1] as AbortSignal;
    if (condition === 'read loss' || condition === 'configure loss') act(() => client.setQueryData(organizationProfileKeys.permissions(ORGANIZATION_TEST_TENANT, ORGANIZATION_TEST_ACTOR), { data: { settings: condition === 'read loss' ? [] : ['read'] } }));
    if (condition === 'logout intent') act(() => { void useAuthStore.getState().logout(); });
    if (condition === 'tenant change') act(() => useAuthStore.setState({ user: { ...user, organization_id: 'c1a6814f-d890-47b6-a3e3-39c1b9aff60f' } }));
    if (condition === 'offline') { vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false); act(() => window.dispatchEvent(new Event('offline'))); }
    if (condition === 'session expiry') act(() => window.dispatchEvent(new Event(SESSION_EXPIRED_EVENT)));
    if (condition === 'unmount') view.unmount();
    await waitFor(() => expect(signal.aborted).toBe(true)); expect(screen.queryByRole('textbox', { name: 'Change reason' })).not.toBeInTheDocument();
    await act(async () => resolve(organizationProfileFixture({ name: 'Late mutation result', version: 5 })));
    expect(screen.queryByText('Late mutation result')).not.toBeInTheDocument();
  });
  it('provides labelled contacts, supported-locale controls and an axe-clean edit surface', async () => {
    render(panel()); await edit(); fireEvent.click(screen.getByRole('button', { name: 'Add informational contact' }));
    expect(screen.getByRole('textbox', { name: 'Contact 2 email' })).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Supported languages' })).toBeInTheDocument();
    expect((await axe(screen.getByRole('dialog'))).violations).toEqual([]);
  });
});
