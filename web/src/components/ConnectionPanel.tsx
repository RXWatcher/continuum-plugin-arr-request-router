import { useState } from "react";
import { api } from "../api/client";
import type { RegisteredArr, Kind } from "../api/types";

export type ConnectionPanelValues = {
  name: string;
  kind: Kind;
  url: string;
  api_key: string;
  root_folder_path: string;
  quality_profile_id?: number;
  language_profile_id?: number;
  priority: number;
  enabled: boolean;
};

type Props = {
  initial?: RegisteredArr;
  arrId?: number;
  onKindChange?: (kind: Kind) => void;
  onSubmit: (values: ConnectionPanelValues) => Promise<void>;
};

function fromInitial(initial?: RegisteredArr): ConnectionPanelValues {
  return {
    name: initial?.name ?? "",
    kind: initial?.kind ?? "radarr",
    url: initial?.url ?? "",
    api_key: "",
    root_folder_path: initial?.root_folder_path ?? "",
    quality_profile_id: initial?.quality_profile_id,
    language_profile_id: initial?.language_profile_id,
    priority: initial?.priority ?? 100,
    enabled: initial?.enabled ?? true,
  };
}

const inputCls =
  "w-full px-3 py-1.5 rounded-md text-sm bg-[var(--surface-raised)] border border-border focus:outline-none focus:ring-1 focus:ring-primary/40 placeholder:text-muted-foreground";
const labelCls = "block text-xs font-medium text-muted-foreground mb-1";

export default function ConnectionPanel({ initial, arrId, onKindChange, onSubmit }: Props) {
  const [vals, setVals] = useState<ConnectionPanelValues>(() => fromInitial(initial));
  const [busy, setBusy] = useState(false);
  const [testMsg, setTestMsg] = useState<{ text: string; ok: boolean } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const set = <K extends keyof ConnectionPanelValues>(k: K, v: ConnectionPanelValues[K]) => {
    setVals((prev) => {
      const next = { ...prev, [k]: v };
      return next;
    });
    if (k === "kind" && onKindChange) {
      onKindChange(v as Kind);
    }
  };

  const validate = (): string | null => {
    if (!vals.name.trim()) return "Name is required.";
    if (!vals.url.trim()) return "URL is required.";
    if (!vals.root_folder_path.trim()) return "Root folder path is required.";
    if (!arrId && !vals.api_key.trim()) return "API key is required for a new *arr.";
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const err = validate();
    if (err) { setError(err); return; }
    setError(null);
    setBusy(true);
    try {
      await onSubmit(vals);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleTest = async () => {
    if (!arrId) return;
    setBusy(true);
    setTestMsg(null);
    try {
      const key = vals.api_key.trim() || undefined;
      const s = await api.testConnection(arrId, key);
      const msg = `${s.appName ?? vals.kind} ${s.version ?? ""} reachable`.trim();
      setTestMsg({ text: msg, ok: true });
    } catch (e: unknown) {
      setTestMsg({ text: e instanceof Error ? e.message : String(e), ok: false });
    } finally {
      setBusy(false);
      setTimeout(() => setTestMsg(null), 4000);
    }
  };

  const isEditing = Boolean(arrId);

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {error && (
        <div className="px-3 py-2 rounded-md text-sm bg-red-900/20 border border-red-700/30 text-red-400">
          {error}
        </div>
      )}

      {/* Name */}
      <div>
        <label className={labelCls}>Name *</label>
        <input
          type="text"
          className={inputCls}
          value={vals.name}
          onChange={(e) => set("name", e.target.value)}
          placeholder="My Radarr"
          required
        />
      </div>

      {/* Kind */}
      <div>
        <label className={labelCls}>Kind</label>
        <div className="flex gap-4 mt-1">
          {(["radarr", "sonarr"] as Kind[]).map((k) => (
            <label key={k} className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="radio"
                name="kind"
                value={k}
                checked={vals.kind === k}
                onChange={() => set("kind", k)}
                className="accent-primary"
              />
              {k}
            </label>
          ))}
        </div>
      </div>

      {/* URL */}
      <div>
        <label className={labelCls}>URL *</label>
        <input
          type="text"
          className={inputCls}
          value={vals.url}
          onChange={(e) => set("url", e.target.value)}
          placeholder="http://radarr:7878"
          required
        />
      </div>

      {/* API Key */}
      <div>
        <label className={labelCls}>API Key {isEditing ? "" : "*"}</label>
        <input
          type="password"
          className={inputCls}
          value={vals.api_key}
          onChange={(e) => set("api_key", e.target.value)}
          placeholder={isEditing ? "(leave blank to keep current)" : "required"}
          autoComplete="new-password"
        />
      </div>

      {/* Root folder path */}
      <div>
        <label className={labelCls}>Root folder path *</label>
        <input
          type="text"
          className={inputCls}
          value={vals.root_folder_path}
          onChange={(e) => set("root_folder_path", e.target.value)}
          placeholder="/movies"
          required
        />
      </div>

      {/* Quality profile ID */}
      <div>
        <label className={labelCls}>Quality profile ID</label>
        <input
          type="number"
          className={inputCls}
          value={vals.quality_profile_id ?? ""}
          onChange={(e) => set("quality_profile_id", e.target.value ? Number(e.target.value) : undefined)}
          placeholder="first available"
          min={1}
        />
      </div>

      {/* Language profile ID (Sonarr only) */}
      {vals.kind === "sonarr" && (
        <div>
          <label className={labelCls}>Language profile ID</label>
          <input
            type="number"
            className={inputCls}
            value={vals.language_profile_id ?? ""}
            onChange={(e) => set("language_profile_id", e.target.value ? Number(e.target.value) : undefined)}
            placeholder="first available"
            min={1}
          />
        </div>
      )}

      {/* Priority */}
      <div>
        <label className={labelCls}>Priority</label>
        <input
          type="number"
          className={`${inputCls} w-32`}
          value={vals.priority}
          onChange={(e) => set("priority", Number(e.target.value))}
          min={0}
        />
        <p className="mt-1 text-xs text-muted-foreground">Lower number = higher priority.</p>
      </div>

      {/* Enabled */}
      <div>
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={vals.enabled}
            onChange={(e) => set("enabled", e.target.checked)}
            className="accent-primary"
          />
          Enabled
        </label>
      </div>

      {/* Buttons */}
      <div className="flex items-center gap-3 pt-1">
        <button
          type="submit"
          disabled={busy}
          className="px-4 py-2 rounded-md text-sm font-medium bg-primary text-primary-foreground hover:brightness-95 transition-all disabled:opacity-50"
        >
          {busy ? "Saving…" : "Save"}
        </button>

        {isEditing && (
          <button
            type="button"
            onClick={handleTest}
            disabled={busy}
            className="px-3 py-2 rounded-md text-sm bg-[var(--surface)] hover:bg-[var(--surface-hover)] border border-border transition-colors disabled:opacity-50"
          >
            Test connection
          </button>
        )}

        {testMsg && (
          <span
            className={`text-sm ${testMsg.ok ? "text-emerald-400" : "text-red-400"}`}
          >
            {testMsg.text}
          </span>
        )}
      </div>
    </form>
  );
}
