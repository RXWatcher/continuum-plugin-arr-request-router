import { useState } from "react";
import { api } from "../api/client";
import type { RouteTestResult } from "../api/types";

export default function RuleTestPanel() {
  const [tmdbId, setTmdbId] = useState<number>(0);
  const [mediaType, setMediaType] = useState<"movie" | "tv">("movie");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<RouteTestResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    if (!tmdbId) return;
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const res = await api.routeTest({ tmdbId, mediaType });
      setResult(res);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const inputCls =
    "px-3 py-1.5 rounded-md text-sm bg-[var(--surface-raised)] border border-border focus:outline-none focus:ring-1 focus:ring-primary/40";

  return (
    <div className="space-y-4">
      <p className="text-xs text-muted-foreground">
        Drives the routing pipeline against your registry without writing anything. Save your rules first.
      </p>

      <div className="flex flex-wrap items-end gap-3">
        {/* TMDB ID */}
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">TMDB ID</label>
          <input
            type="number"
            className={`${inputCls} w-32`}
            value={tmdbId || ""}
            onChange={(e) => setTmdbId(e.target.valueAsNumber || 0)}
            placeholder="e.g. 550"
            min={1}
          />
        </div>

        {/* Media type */}
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">Media type</label>
          <select
            className={inputCls}
            value={mediaType}
            onChange={(e) => setMediaType(e.target.value as "movie" | "tv")}
          >
            <option value="movie">movie</option>
            <option value="tv">tv</option>
          </select>
        </div>

        {/* Run */}
        <button
          type="button"
          onClick={run}
          disabled={busy || !tmdbId}
          className="px-4 py-1.5 rounded-md text-sm font-medium bg-primary text-primary-foreground hover:brightness-95 transition-all disabled:opacity-50"
        >
          {busy ? "Running…" : "Run"}
        </button>
      </div>

      {/* Error */}
      {error && (
        <div className="px-3 py-2 rounded-md text-sm bg-red-900/20 border border-red-700/30 text-red-400">
          {error}
        </div>
      )}

      {/* Result */}
      {result && (
        <div className="space-y-2">
          <div className="text-sm">
            <span className="text-muted-foreground">Chosen: </span>
            <span className="font-mono">
              {result.chosen !== null ? result.chosen : "null (unrouted)"}
            </span>
          </div>
          <pre className="text-xs p-3 rounded-lg bg-[var(--surface)] border border-border overflow-x-auto max-h-64 text-muted-foreground">
            {JSON.stringify(result.trace, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}
