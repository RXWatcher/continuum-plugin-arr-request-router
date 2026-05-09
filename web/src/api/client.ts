import type { RegisteredArr, RequestRow, RouteTestResult, Rules, SystemStatus } from "./types";

const base = "/api/admin";

async function jsonOrThrow<T>(r: Response): Promise<T> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text().catch(() => "")}`);
  return r.json();
}

async function noContentOrThrow(r: Response): Promise<void> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text().catch(() => "")}`);
}

export const api = {
  listArrs: () =>
    fetch(`${base}/registry`).then(jsonOrThrow<RegisteredArr[]>),

  getArr: (id: number) =>
    fetch(`${base}/registry/${id}`).then(jsonOrThrow<RegisteredArr>),

  createArr: (input: Partial<RegisteredArr> & { api_key: string; rules: Rules }) =>
    fetch(`${base}/registry`, {
      method: "POST",
      body: JSON.stringify(input),
      headers: { "Content-Type": "application/json" },
    }).then(jsonOrThrow<{ id: number }>),

  updateArr: (id: number, patch: Partial<RegisteredArr> & { api_key?: string }) =>
    fetch(`${base}/registry/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
      headers: { "Content-Type": "application/json" },
    }).then(noContentOrThrow),

  deleteArr: (id: number) =>
    fetch(`${base}/registry/${id}`, { method: "DELETE" }).then(noContentOrThrow),

  testConnection: (id: number, api_key?: string) =>
    fetch(`${base}/registry/${id}/test-connection`, {
      method: "POST",
      body: JSON.stringify({ api_key }),
      headers: { "Content-Type": "application/json" },
    }).then(jsonOrThrow<SystemStatus>),

  routeTest: (input: { tmdbId: number; mediaType: "movie"|"tv"; title?: string; year?: number }) =>
    fetch(`${base}/route-test`, {
      method: "POST",
      body: JSON.stringify(input),
      headers: { "Content-Type": "application/json" },
    }).then(jsonOrThrow<RouteTestResult>),

  listRequests: (p: { status?: string; page?: number; limit?: number }) => {
    const q = new URLSearchParams();
    if (p.status) q.set("status", p.status);
    if (p.page)   q.set("page", String(p.page));
    if (p.limit)  q.set("limit", String(p.limit));
    return fetch(`${base}/requests?${q.toString()}`).then(jsonOrThrow<{ rows: RequestRow[]; total: number }>);
  },

  getRequest: (id: string) =>
    fetch(`${base}/requests/${id}`).then(jsonOrThrow<RequestRow>),

  retryRequest: (id: string) =>
    fetch(`${base}/requests/${id}/retry`, { method: "POST" }).then(noContentOrThrow),

  reRouteRequest: (id: string) =>
    fetch(`${base}/requests/${id}/re-route`, { method: "POST" }).then(noContentOrThrow),
};
