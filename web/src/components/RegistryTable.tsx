import { useEffect, useState } from "react";
import { Link } from "react-router";
import { api } from "../api/client";
import type { RegisteredArr } from "../api/types";

function KindPill({ kind }: { kind: RegisteredArr["kind"] }) {
  const cls =
    kind === "radarr"
      ? "bg-amber-500/15 text-amber-300 border-amber-500/30"
      : "bg-sky-500/15 text-sky-300 border-sky-500/30";
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs border ${cls}`}>
      {kind}
    </span>
  );
}

export default function RegistryTable() {
  const [rows, setRows] = useState<RegisteredArr[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<number | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  const reload = async () => {
    setError(null);
    try {
      setRows(await api.listArrs());
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setError(msg);
    }
  };

  useEffect(() => {
    reload();
  }, []);

  const showToast = (msg: string, ok: boolean) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 4000);
  };

  const toggleEnabled = async (a: RegisteredArr) => {
    setBusy(a.id);
    try {
      await api.updateArr(a.id, { enabled: !a.enabled });
      await reload();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showToast(`Failed to update: ${msg}`, false);
    } finally {
      setBusy(null);
    }
  };

  const remove = async (a: RegisteredArr) => {
    if (
      !window.confirm(
        `Delete ${a.name}? In-flight requests routed to this *arr will become orphaned.`
      )
    )
      return;
    setBusy(a.id);
    try {
      await api.deleteArr(a.id);
      await reload();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showToast(`Failed to delete: ${msg}`, false);
    } finally {
      setBusy(null);
    }
  };

  const probe = async (a: RegisteredArr) => {
    setBusy(a.id);
    try {
      const s = await api.testConnection(a.id);
      showToast(`${s.appName ?? a.kind} ${s.version ?? ""} reachable`.trim(), true);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      showToast(`Failed: ${msg}`, false);
    } finally {
      setBusy(null);
    }
  };

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

      {/* Loading */}
      {rows === null && !error && (
        <div className="py-12 text-center text-muted-foreground text-sm">Loading…</div>
      )}

      {/* Error */}
      {error && (
        <div className="py-8 text-center">
          <p className="text-red-400 text-sm mb-3">{error}</p>
          <button
            onClick={reload}
            className="px-3 py-1.5 text-sm rounded-md bg-[var(--surface)] hover:bg-[var(--surface-hover)] text-foreground border border-border transition-colors"
          >
            Retry
          </button>
        </div>
      )}

      {/* Empty */}
      {rows !== null && rows.length === 0 && !error && (
        <div className="py-12 text-center text-muted-foreground text-sm">
          No registered *arrs yet.{" "}
          <Link to="/registry/new" className="text-primary hover:underline">
            Add one
          </Link>
          .
        </div>
      )}

      {/* Table */}
      {rows !== null && rows.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-[var(--surface)]">
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Name
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Kind
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  URL
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Priority
                </th>
                <th className="px-4 py-3 text-center text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Enabled
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a, i) => (
                <tr
                  key={a.id}
                  className={[
                    "border-b border-border last:border-0 transition-colors",
                    i % 2 === 0 ? "bg-background" : "bg-[var(--surface)]",
                    "hover:bg-[var(--surface-hover)]",
                  ].join(" ")}
                >
                  <td className="px-4 py-3 font-medium">{a.name}</td>
                  <td className="px-4 py-3">
                    <KindPill kind={a.kind} />
                  </td>
                  <td className="px-4 py-3 max-w-[220px]">
                    <span
                      className="font-mono text-xs text-muted-foreground truncate block"
                      title={a.url}
                    >
                      {a.url}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums">{a.priority}</td>
                  <td className="px-4 py-3 text-center">
                    <button
                      onClick={() => toggleEnabled(a)}
                      disabled={busy === a.id}
                      title={a.enabled ? "Disable" : "Enable"}
                      className={[
                        "relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none disabled:opacity-50",
                        a.enabled ? "bg-primary" : "bg-[var(--surface-raised,#28282e)]",
                      ].join(" ")}
                    >
                      <span
                        className={[
                          "inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform",
                          a.enabled ? "translate-x-[18px]" : "translate-x-[3px]",
                        ].join(" ")}
                      />
                    </button>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Link
                        to={`/registry/${a.id}`}
                        className="px-2.5 py-1 rounded text-xs bg-[var(--surface)] hover:bg-[var(--surface-hover)] border border-border transition-colors"
                      >
                        Edit
                      </Link>
                      <button
                        onClick={() => probe(a)}
                        disabled={busy === a.id}
                        className="px-2.5 py-1 rounded text-xs bg-[var(--surface)] hover:bg-[var(--surface-hover)] border border-border transition-colors disabled:opacity-50"
                      >
                        Test
                      </button>
                      <button
                        onClick={() => remove(a)}
                        disabled={busy === a.id}
                        className="px-2.5 py-1 rounded text-xs bg-red-900/20 hover:bg-red-900/40 border border-red-700/30 text-red-400 transition-colors disabled:opacity-50"
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
