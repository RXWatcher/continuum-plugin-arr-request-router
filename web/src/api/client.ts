import type { AppConfig, RegisteredArr, RequestRow, RouteTestResult, Rules, SystemStatus } from './types';
import { mountPath } from '../lib/mountPath';
import { getCachedToken } from '../lib/auth';

function apiBase(): string {
  return `${mountPath()}/api/admin`;
}

function authHeaders(): Record<string, string> {
  const t = getCachedToken();
  return t ? { Authorization: `Bearer ${t}` } : {};
}

async function authedFetch(input: string, init?: RequestInit): Promise<Response> {
  const headers = {
    ...(init?.headers as Record<string, string> | undefined),
    ...authHeaders(),
  };
  return fetch(input, { ...init, headers });
}

async function jsonOrThrow<T>(r: Response): Promise<T> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text().catch(() => '')}`);
  return r.json();
}

async function noContentOrThrow(r: Response): Promise<void> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text().catch(() => '')}`);
}

export const api = {
  config: () => authedFetch(`${apiBase()}/config`).then(jsonOrThrow<AppConfig>),

  updateConfig: (config: AppConfig) =>
    authedFetch(`${apiBase()}/config`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    }).then(jsonOrThrow<AppConfig>),

  listArrs: () => authedFetch(`${apiBase()}/registry`).then(jsonOrThrow<RegisteredArr[]>),

  getArr: (id: number) =>
    authedFetch(`${apiBase()}/registry/${id}`).then(jsonOrThrow<RegisteredArr>),

  createArr: (input: Partial<RegisteredArr> & { api_key: string; rules: Rules }) =>
    authedFetch(`${apiBase()}/registry`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(jsonOrThrow<RegisteredArr & { id: number }>),

  updateArr: (id: number, patch: Partial<RegisteredArr> & { api_key?: string }) =>
    authedFetch(`${apiBase()}/registry/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    }).then(noContentOrThrow),

  deleteArr: (id: number) =>
    authedFetch(`${apiBase()}/registry/${id}`, { method: 'DELETE' }).then(noContentOrThrow),

  testConnection: (id: number, api_key?: string) =>
    authedFetch(`${apiBase()}/registry/${id}/test-connection`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ api_key }),
    }).then(jsonOrThrow<SystemStatus>),

  routeTest: (input: { tmdbId: number; mediaType: 'movie' | 'tv'; title?: string; year?: number }) =>
    authedFetch(`${apiBase()}/route-test`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(jsonOrThrow<RouteTestResult>),

  listRequests: (p: { status?: string; page?: number; limit?: number }) => {
    const q = new URLSearchParams();
    if (p.status) q.set('status', p.status);
    if (p.page) q.set('page', String(p.page));
    if (p.limit) q.set('limit', String(p.limit));
    return authedFetch(`${apiBase()}/requests?${q.toString()}`).then(
      jsonOrThrow<{ rows: RequestRow[]; total: number }>,
    );
  },

  getRequest: (id: string) =>
    authedFetch(`${apiBase()}/requests/${id}`).then(jsonOrThrow<RequestRow>),

  retryRequest: (id: string) =>
    authedFetch(`${apiBase()}/requests/${id}/retry`, { method: 'POST' }).then(noContentOrThrow),

  reRouteRequest: (id: string) =>
    authedFetch(`${apiBase()}/requests/${id}/re-route`, { method: 'POST' }).then(noContentOrThrow),

  forceFailRequest: (id: string) =>
    authedFetch(`${apiBase()}/requests/${id}/force-fail`, { method: 'POST' }).then(
      noContentOrThrow,
    ),
};

// Re-exported for ad-hoc tests of URL composition.
export const _internals = { apiBase };
