import { describe, it, expect } from 'vitest';
import { extractMountPath } from './mountPath';

describe('extractMountPath', () => {
  it('extracts /api/v1/plugins/18 from a deep link', () => {
    expect(extractMountPath('/api/v1/plugins/18/admin/registry/123/edit')).toBe(
      '/api/v1/plugins/18',
    );
  });
  it('extracts /api/v1/plugins/1 from the SPA root path', () => {
    expect(extractMountPath('/api/v1/plugins/1/admin')).toBe('/api/v1/plugins/1');
  });
  it('returns empty string when the prefix is not present (dev server case)', () => {
    expect(extractMountPath('/admin/registry')).toBe('');
  });
  it('returns empty string for root', () => {
    expect(extractMountPath('/')).toBe('');
  });
});
