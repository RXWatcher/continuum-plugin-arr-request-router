import { describe, it, expect, vi, beforeEach } from 'vitest';

describe('apiBase composition', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('produces /api/v1/plugins/<id>/api/admin under the plugin proxy', async () => {
    Object.defineProperty(window, 'location', {
      value: { pathname: '/api/v1/plugins/42/admin/registry/7/edit', search: '' },
      writable: true,
    });
    const { _internals } = await import('./client');
    expect(_internals.apiBase()).toBe('/api/v1/plugins/42/api/admin');
  });

  it('falls back to /api/admin on the dev server (no proxy prefix)', async () => {
    Object.defineProperty(window, 'location', {
      value: { pathname: '/admin', search: '' },
      writable: true,
    });
    const { _internals } = await import('./client');
    expect(_internals.apiBase()).toBe('/api/admin');
  });
});
