import { describe, it, expect, beforeEach } from 'vitest';
import { captureFromURL, getCachedToken, getCachedTheme, _resetForTest } from './auth';

describe('captureFromURL', () => {
  beforeEach(() => {
    _resetForTest();
    sessionStorage.clear();
  });

  it('captures token and theme from URL search params', () => {
    captureFromURL(new URLSearchParams('?token=abc123&theme=midnight'));
    expect(getCachedToken()).toBe('abc123');
    expect(getCachedTheme()).toBe('midnight');
  });

  it('falls back to sessionStorage for theme when URL lacks ?theme=', () => {
    sessionStorage.setItem('silo-theme', 'arctic');
    captureFromURL(new URLSearchParams('?token=abc'));
    expect(getCachedTheme()).toBe('arctic');
  });

  it('persists captured theme to sessionStorage', () => {
    captureFromURL(new URLSearchParams('?theme=cobalt'));
    expect(sessionStorage.getItem('silo-theme')).toBe('cobalt');
  });

  it('returns null when neither token nor theme is provided', () => {
    captureFromURL(new URLSearchParams());
    expect(getCachedToken()).toBeNull();
  });
});
