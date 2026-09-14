import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { Asset } from '@/types/asset';
import { AssetDeleteDialog } from '@/components/assets/asset-delete-dialog';
import userEvent from '@testing-library/user-event';

const mutateAsync = vi.fn();
vi.mock('@/lib/api-hooks', () => ({ useDeleteAsset: () => ({ mutateAsync, isPending: false, error: null }) }));
const ASSET = { id: 'asset-1', asset_ref: 'AST-2026-0001', version: 4 } as Asset;

describe('AssetDeleteDialog', () => {
  beforeEach(() => mutateAsync.mockReset().mockResolvedValue(undefined));
  it('announces the consequence and requires the exact asset reference', async () => {
    const user = userEvent.setup();
    const onDeleted = vi.fn();
    render(<AssetDeleteDialog asset={ASSET} open onOpenChange={vi.fn()} onDeleted={onDeleted} onRefresh={vi.fn()} />);
    expect(screen.getByRole('alertdialog', { name: 'Delete asset?' })).toHaveAccessibleDescription(/soft-deletes the inventory record/i);
    const button = screen.getByRole('button', { name: 'Delete asset' });
    expect(button).toBeDisabled();
    await user.type(screen.getByLabelText(/Type AST-2026-0001 to confirm/), 'AST-2026-0001');
    await user.click(button);
    expect(mutateAsync).toHaveBeenCalledWith({ id: 'asset-1', expectedVersion: 4 });
    expect(onDeleted).toHaveBeenCalledOnce();
  });
});
