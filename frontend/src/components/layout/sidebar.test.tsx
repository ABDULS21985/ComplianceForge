import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { NAVIGATION_GROUPS } from '@/lib/navigation';
import { SidebarContent } from '@/components/layout/sidebar';
import { useNavigationStore } from '@/store/navigation-store';
import userEvent from '@testing-library/user-event';

const superAdminContext = {
  isSuperAdmin: true,
  permissions: {},
  roleSlugs: [],
};

beforeEach(() => {
  useNavigationStore.setState({
    collapsed: false,
    expandedGroups: Object.fromEntries(
      NAVIGATION_GROUPS.map((group) => [group.id, Boolean(group.defaultOpen)])
    ),
    favoriteIds: [],
    recentIds: [],
  });
  window.localStorage.clear();
});

describe('sidebar navigation', () => {
  it('exposes grouped, collapsible navigation and the active page', async () => {
    const user = userEvent.setup();
    render(
      <SidebarContent
        collapsed={false}
        context={superAdminContext}
        pathname="/risks"
      />
    );

    const group = screen.getByRole('button', { name: 'Risk & Resilience' });
    expect(group).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'Risk Register' })).toHaveAttribute(
      'aria-current',
      'page'
    );

    await user.click(group);
    expect(group).toHaveAttribute('aria-expanded', 'false');
    expect(
      screen.queryByRole('link', { name: 'Risk Register' })
    ).not.toBeInTheDocument();
  });

  it('persists a user-selected favorite as a shortcut', async () => {
    const user = userEvent.setup();
    render(
      <SidebarContent
        collapsed={false}
        context={superAdminContext}
        pathname="/dashboard"
      />
    );

    await user.click(
      screen.getByRole('button', { name: 'Add Frameworks to favorites' })
    );

    expect(screen.getByRole('heading', { name: 'Favorites' })).toBeVisible();
    expect(
      screen.getAllByRole('link', { name: 'Frameworks' })
    ).toHaveLength(2);
    expect(useNavigationStore.getState().favoriteIds).toContain('frameworks');
  });
});
