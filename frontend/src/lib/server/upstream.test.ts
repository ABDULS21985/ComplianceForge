import { afterEach, describe, expect, it } from 'vitest';

import { buildUpstreamUrl, internalApiBaseUrl } from '@/lib/server/upstream';

const originalInternalUrl = process.env.API_INTERNAL_URL;

afterEach(() => {
  if (originalInternalUrl === undefined) delete process.env.API_INTERNAL_URL;
  else process.env.API_INTERNAL_URL = originalInternalUrl;
});

describe('internal API URL contract', () => {
  it('canonicalizes a valid server-only base URL', () => {
    process.env.API_INTERNAL_URL = 'http://api:8080/api/v1/';

    expect(internalApiBaseUrl()).toBe('http://api:8080/api/v1');
    expect(buildUpstreamUrl('/risks', '?page=2')).toBe(
      'http://api:8080/api/v1/risks?page=2',
    );
  });

  it.each([
    'ftp://api.example.test/api/v1',
    'https://user:secret@api.example.test/api/v1',
    'https://api.example.test/api/v1?tenant=other',
    'https://api.example.test/api/v1#fragment',
    'https://api.example.test/api//v1',
  ])('rejects unsafe or non-canonical value %s', (value) => {
    process.env.API_INTERNAL_URL = value;
    expect(() => internalApiBaseUrl()).toThrow();
  });
});
