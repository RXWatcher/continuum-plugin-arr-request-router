import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { RequestRow, Status } from "../api/types";
import StatusPill from "./StatusPill";

const ALL_STATUSES: Status[] = [
  "queued",
  "submitted",
  "downloading",
  "imported",
  "failed",
  "cancelled",
  "unrouted",
];

const LIMIT = 25;

function truncate(s: string | undefined, max: number): string {
  if (!s) return "";
  return s.length > max ? s.slice(0, max) + "…" : s;
}

export default function RequestsQueue() {
  const [rows, setRows] = useState<RequestRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState<Status | "">("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  const showToast = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 4000);
  };

  const reload = async (p: number = page, status: Status | "" = statusFilter) => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.listRequests({
        status: status || undefined,
        page: p,
        limit: LIMIT,
      });
      setRows(result.rows);
      setTotal(result.total);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    reload(page, statusFilter);
  }, [page, statusFilter]);

  const handleStatusChange = (value: string) => {
    setPage(1);
    setStatusFilter(value as Status | "");
  };

  const retry = async (row: RequestRow) => {
    setBusy(row.id);
    try {
      await api.retryRequest(row.id);
      showToast("Retry queued", true);
      await reload();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showToast(`Retry failed: ${msg}`, false);
    } finally {
      setBusy(null);
    }
  };

  const reRoute = async (row: RequestRow) => {
    setBusy(row.id);
    try {
      await api.reRouteRequest(row.id);
      showToast("Re-route queued", true);
      await reload();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showToast(`Re-route failed: ${msg}`, false);
    } finally {
      setBusy(null);
    }
  };

  const forceFail = async (row: RequestRow) => {
    if (
      !window.confirm(
        `Force-fail ${row.title}? This is for rows stuck after a registered *arr was deleted.`
      )
    )
      return;
    setBusy(row.id);
    try {
      await api.forceFailRequest(row.id);
      showToast(`Force-failed ${row.title}`, true);
      await reload();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showToast(`Force-fail failed: ${msg}`, false);
    } finally {
      setBusy(null);
    }
  };

  const hasPrev = page > 1;
  const hasNext = page * LIMIT < total;

  return (
    <div className="relative">
      {/* Toast */}
      {toast && (
        <div
          className={[
            "fixed bottom-6 right-6 z-50 px-4 py-3 rounded-lg text-sm shadow-lg border",
            toast.ok
              ? "bg-emerald-900/80 text-emerald-200 border-emerald-700"
              : "bg-red-900/80 text-red-200 border-red-700",
          ].join(" ")}
        >
          {toast.msg}
        </div>
      )}

      {/* Filter bar */}
      <div className="flex items-center gap-3 mb-4">
        <label className="text-sm text-muted-foreground" htmlFor="status-filter">
          Status
        </label>
        <select
          id="status-filter"
          value={statusFilter}
          onChange={(e) => handleStatusChange(e.target.value)}
          className="px-3 py-1.5 text-sm rounded-md border border-border bg-[var(--surface)] text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
        >
          <option value="">All</option>
          {ALL_STATUSES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <span className="text-xs text-muted-foreground ml-auto">
          {total} request{total !== 1 ? "s" : ""}
        </span>
      </div>

      {/* Loading */}
      {loading && (
        <div className="py-12 text-center text-muted-foreground text-sm">Loading…</div>
      )}

      {/* Error */}
      {!loading && error && (
        <div className="py-8 text-center">
          <p className="text-red-400 text-sm mb-3">{error}</p>
          <button
            onClick={() => reload()}
            className="px-3 py-1.5 text-sm rounded-md bg-[var(--surface)] hover:bg-[var(--surface-hover)] text-foreground border border-border transition-colors"
          >
            Retry
          </button>
        </div>
      )}

      {/* Empty */}
      {!loading && !error && rows.length === 0 && (
        <div className="py-12 text-center text-muted-foreground text-sm">
          No requests found.
        </div>
      )}

      {/* Table */}
      {!loading && !error && rows.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-[var(--surface)]">
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  ID
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Title
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Kind
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Status
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Routed *arr
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Error
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Created
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                <tr
                  key={row.id}
                  className={[
                    "border-b border-border last:border-0 transition-colors",
                    i % 2 === 0 ? "bg-background" : "bg-[var(--surface)]",
                    "hover:bg-[var(--surface-hover)]",
                  ].join(" ")}
                >
                  <td className="px-4 py-3">
                    <span
                      className="font-mono text-xs text-muted-foreground"
                      title={row.id}
                    >
                      {truncate(row.id, 12)}
                    </span>
                  </td>
                  <td className="px-4 py-3 max-w-[200px]">
                    <span className="block truncate font-medium" title={row.title}>
                      {row.title}
                    </span>
                    {row.year && (
                      <span className="text-xs text-muted-foreground">{row.year}</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground text-xs">
                    {row.media_type}
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill status={row.status} />
                  </td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">
                    {row.routed_arr_name ?? "—"}
                  </td>
                  <td className="px-4 py-3 max-w-[180px]">
                    {row.error ? (
                      <span
                        className="text-xs text-red-400 truncate block cursor-help"
                        title={row.error}
                      >
                        {truncate(row.error, 40)}
                      </span>
                    ) : (
                      <span className="text-muted-foreground text-xs">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground whitespace-nowrap">
                    {new Date(row.created_at).toLocaleString(undefined, {
                      month: "short",
                      day: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-2">
                      {row.status === "failed" && (
                        <button
                          onClick={() => retry(row)}
                          disabled={busy === row.id}
                          className="px-2.5 py-1 rounded text-xs bg-[var(--surface)] hover:bg-[var(--surface-hover)] border border-border transition-colors disabled:opacity-50"
                        >
                          Retry
                        </button>
                      )}
                      {row.status === "unrouted" && (
                        <button
                          onClick={() => reRoute(row)}
                          disabled={busy === row.id}
                          className="px-2.5 py-1 rounded text-xs bg-[var(--surface)] hover:bg-[var(--surface-hover)] border border-border transition-colors disabled:opacity-50"
                        >
                          Re-route
                        </button>
                      )}
                      {(row.status === "submitted" || row.status === "downloading") && (
                        <button
                          type="button"
                          onClick={() => forceFail(row)}
                          disabled={busy === row.id}
                          className="text-sm text-red-400 hover:underline disabled:opacity-50"
                        >
                          Force fail
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination */}
      {!loading && !error && total > LIMIT && (
        <div className="flex items-center justify-between mt-4 text-sm">
          <button
            onClick={() => setPage((p) => p - 1)}
            disabled={!hasPrev}
            className="px-3 py-1.5 rounded-md border border-border bg-[var(--surface)] hover:bg-[var(--surface-hover)] text-foreground transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            ← Prev
          </button>
          <span className="text-muted-foreground text-xs">
            Page {page} of {Math.ceil(total / LIMIT)}
          </span>
          <button
            onClick={() => setPage((p) => p + 1)}
            disabled={!hasNext}
            className="px-3 py-1.5 rounded-md border border-border bg-[var(--surface)] hover:bg-[var(--surface-hover)] text-foreground transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Next →
          </button>
        </div>
      )}
    </div>
  );
}
