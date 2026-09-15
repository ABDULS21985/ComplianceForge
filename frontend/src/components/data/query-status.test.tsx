import { describe, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { axe } from 'jest-axe';
import { QueryStatus } from '@/components/data/query-status';
import userEvent from '@testing-library/user-event';

function renderWithClient(
  query: () => Promise<string>,
  options: { initialData?: string; retry?: boolean } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: options.retry ?? false } },
  });

  function QueryConsumer() {
    useQuery({
      initialData: options.initialData,
      queryKey: ['query-status-test'],
      queryFn: query,
    });
    return null;
  }

  return render(
    <QueryClientProvider client={client}>
      <QueryStatus />
      <QueryConsumer />
    </QueryClientProvider>,
  );
}

describe('global query status', () => {
  it('announces active loading without exposing a visual spinner storm', async () => {
    renderWithClient(() => new Promise(() => undefined));

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(
        'Loading 1 information source.',
      ),
    );
  });

  it('renders a safe axe-clean error and retries only active failed data', async () => {
    const query = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new Error('sensitive upstream detail'))
      .mockResolvedValueOnce('ready');
    const user = userEvent.setup();
    const { container } = renderWithClient(query);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Some page information could not be loaded');
    expect(alert).not.toHaveTextContent('sensitive upstream detail');
    expect((await axe(container)).violations).toEqual([]);

    await user.click(screen.getByRole('button', { name: 'Retry page data' }));
    await waitFor(() => expect(query).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
  });

  it('does not offer a retry for a permission denial', async () => {
    renderWithClient(() => Promise.reject({ status: 403 }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Some information is restricted',
    );
    expect(
      screen.queryByRole('button', { name: 'Retry page data' }),
    ).not.toBeInTheDocument();
  });

  it('distinguishes a failed refresh with retained data from an initial load failure', async () => {
    renderWithClient(() => Promise.reject(new Error('withheld')), {
      initialData: 'previous snapshot',
    });

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Some saved information may be out of date',
    );
  });
});
