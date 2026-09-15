import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { axe } from 'jest-axe';
import { DataTable } from '@/components/data/data-table';
import userEvent from '@testing-library/user-event';

describe('DataTable accessibility', () => {
  it('names the table and controls, exposes sort state, and provides a keyboard row action', async () => {
    const openRow = vi.fn();
    const user = userEvent.setup();
    const { container } = render(
      <DataTable
        accessibleLabel="Vendor register"
        columns={[
          { key: 'name', label: 'Vendor', sortable: true },
          { key: 'status', label: 'Status' },
        ]}
        data={[{ id: 'vendor-1', name: 'Acme Services', status: 'Active' }]}
        onRowClick={openRow}
        onSearch={vi.fn()}
        searchPlaceholder="Search vendors"
      />,
    );

    expect(screen.getByRole('table', { name: 'Vendor register' })).toBeVisible();
    expect(screen.getByRole('textbox', { name: 'Search vendors' })).toBeVisible();

    const sort = screen.getByRole('button', { name: 'Vendor' });
    await user.click(sort);
    expect(screen.getByRole('columnheader', { name: 'Vendor' })).toHaveAttribute(
      'aria-sort',
      'ascending',
    );
    await user.click(sort);
    expect(screen.getByRole('columnheader', { name: 'Vendor' })).toHaveAttribute(
      'aria-sort',
      'descending',
    );

    const rowAction = screen.getByRole('button', { name: 'Open Acme Services' });
    rowAction.focus();
    await user.keyboard('{Enter}');
    expect(openRow).toHaveBeenCalledOnce();
    expect((await axe(container)).violations).toEqual([]);
  });
});
