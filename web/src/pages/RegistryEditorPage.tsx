import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { api } from "../api/client";
import type { RegisteredArr, Kind, Rules } from "../api/types";
import { normalizeRules, emptyRules } from "../lib/rules";
import ConnectionPanel, { type ConnectionPanelValues } from "../components/ConnectionPanel";
import CollectionBuilder from "../components/CollectionBuilder";
import RuleTestPanel from "../components/RuleTestPanel";

export default function RegistryEditorPage() {
  const { id } = useParams<{ id?: string }>();
  const arrId = id && id !== "new" ? Number(id) : undefined;
  const navigate = useNavigate();

  const [arr, setArr] = useState<RegisteredArr | undefined>(undefined);
  const [rules, setRules] = useState<Rules>(emptyRules());
  const [kind, setKind] = useState<Kind>("radarr");
  const [loading, setLoading] = useState(Boolean(arrId));
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!arrId) return;
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    api.getArr(arrId)
      .then((data) => {
        if (cancelled) return;
        setArr(data);
        setKind(data.kind);
        setRules(normalizeRules(data.rules));
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setLoadError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [arrId]);

  const handleSubmit = async (connValues: ConnectionPanelValues) => {
    setSaveError(null);
    try {
      if (arrId) {
        // Edit
        const patch: Partial<RegisteredArr> & { api_key?: string; rules: Rules } = {
          name: connValues.name,
          kind: connValues.kind,
          url: connValues.url,
          root_folder_path: connValues.root_folder_path,
          quality_profile_id: connValues.quality_profile_id,
          language_profile_id: connValues.language_profile_id,
          priority: connValues.priority,
          enabled: connValues.enabled,
          rules,
        };
        if (connValues.api_key.trim()) {
          patch.api_key = connValues.api_key.trim();
        }
        await api.updateArr(arrId, patch);
      } else {
        // Create
        const res = await api.createArr({
          name: connValues.name,
          kind: connValues.kind,
          url: connValues.url,
          api_key: connValues.api_key,
          root_folder_path: connValues.root_folder_path,
          quality_profile_id: connValues.quality_profile_id,
          language_profile_id: connValues.language_profile_id,
          priority: connValues.priority,
          enabled: connValues.enabled,
          rules,
        });
        navigate(`/registry/${res.id}`, { replace: true });
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setSaveError(msg);
      throw e; // re-throw so ConnectionPanel can clear its busy state
    }
  };

  if (loading) {
    return (
      <div className="py-12 text-center text-muted-foreground text-sm">Loading…</div>
    );
  }

  if (loadError) {
    return (
      <div className="py-12 text-center space-y-3">
        <p className="text-red-400 text-sm">{loadError}</p>
        <Link to="/" className="text-primary hover:underline text-sm">
          ← Back to registry
        </Link>
      </div>
    );
  }

  const isNew = !arrId;
  const heading = isNew ? "New registered *arr" : `Edit ${arr?.name ?? "…"}`;

  return (
    <div className="max-w-2xl mx-auto py-8 space-y-8">
      {/* Back link */}
      <Link to="/" className="text-sm text-muted-foreground hover:text-foreground transition-colors">
        ← Back to registry
      </Link>

      {/* Heading */}
      <h2 className="text-xl font-semibold">{heading}</h2>

      {/* Top-level save error (set by this page, not by ConnectionPanel) */}
      {saveError && (
        <div className="px-3 py-2 rounded-md text-sm bg-red-900/20 border border-red-700/30 text-red-400">
          {saveError}
        </div>
      )}

      {/* Connection panel */}
      <section className="rounded-lg border border-border bg-[var(--surface)] p-5 space-y-1">
        <ConnectionPanel
          initial={arr}
          arrId={arrId}
          onKindChange={setKind}
          onSubmit={handleSubmit}
        />
      </section>

      {/* Routing rules */}
      <section className="space-y-3">
        <h3 className="text-base font-medium">Routing rules</h3>
        <CollectionBuilder value={rules} onChange={setRules} kind={kind} />
      </section>

      {/* Test rules panel — only when editing */}
      {arrId && (
        <section className="rounded-lg border border-border bg-[var(--surface)] p-5 space-y-2">
          <h3 className="text-base font-medium mb-1">Test routing</h3>
          <RuleTestPanel />
        </section>
      )}
    </div>
  );
}
