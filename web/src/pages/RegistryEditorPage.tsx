import { useState, useEffect } from "react";
import { useNavigate, useParams, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Plug, Save } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api/client";
import type { Kind, RegisteredArr, Rules } from "@/api/types";
import { emptyRules, normalizeRules } from "@/lib/rules";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type FormState = {
  name: string;
  kind: Kind;
  url: string;
  api_key: string;
  root_folder_path: string;
  quality_profile_id?: number;
  language_profile_id?: number;
  priority: number;
  enabled: boolean;
  rules: Rules;
};

const empty: FormState = {
  name: "",
  kind: "radarr",
  url: "",
  api_key: "",
  root_folder_path: "",
  priority: 100,
  enabled: true,
  rules: emptyRules(),
};

export default function RegistryEditorPage() {
  const { id } = useParams<{ id: string }>();
  const isNew = !id || id === "new";
  const arrId = isNew ? undefined : Number(id);
  const navigate = useNavigate();
  const qc = useQueryClient();

  const { data: existing, isLoading } = useQuery({
    queryKey: ["arr", arrId],
    queryFn: () => api.getArr(arrId!),
    enabled: !isNew,
  });

  const [form, setForm] = useState<FormState>(empty);

  useEffect(() => {
    if (!existing) return;
    setForm({
      name: existing.name,
      kind: existing.kind,
      url: existing.url,
      api_key: "",
      root_folder_path: existing.root_folder_path ?? "",
      quality_profile_id: existing.quality_profile_id,
      language_profile_id: existing.language_profile_id,
      priority: existing.priority,
      enabled: existing.enabled,
      rules: normalizeRules(existing.rules),
    });
  }, [existing]);

  const saveMutation = useMutation({
    mutationFn: async (): Promise<RegisteredArr | undefined> => {
      if (!form.name.trim()) throw new Error("Name is required.");
      if (!form.url.trim()) throw new Error("URL is required.");
      if (isNew && !form.api_key.trim()) throw new Error("API key is required for new registries.");

      if (arrId) {
        const patch: Partial<RegisteredArr> & { api_key?: string; rules: Rules } = {
          name: form.name,
          kind: form.kind,
          url: form.url,
          root_folder_path: form.root_folder_path,
          quality_profile_id: form.quality_profile_id,
          language_profile_id: form.language_profile_id,
          priority: form.priority,
          enabled: form.enabled,
          rules: form.rules,
        };
        if (form.api_key.trim()) patch.api_key = form.api_key.trim();
        await api.updateArr(arrId, patch);
        return undefined;
      }
      return api.createArr({
        name: form.name,
        kind: form.kind,
        url: form.url,
        api_key: form.api_key.trim(),
        root_folder_path: form.root_folder_path,
        quality_profile_id: form.quality_profile_id,
        language_profile_id: form.language_profile_id,
        priority: form.priority,
        enabled: form.enabled,
        rules: form.rules,
      });
    },
    onSuccess: (saved) => {
      qc.invalidateQueries({ queryKey: ["arrs"] });
      if (saved) {
        qc.invalidateQueries({ queryKey: ["arr", saved.id] });
        if (isNew) {
          navigate(`/registry/${saved.id}`, { replace: true });
        }
      } else if (arrId) {
        qc.invalidateQueries({ queryKey: ["arr", arrId] });
      }
      toast.success("Saved");
    },
    onError: (e: Error) => {
      toast.error(`Save failed: ${e.message}`);
    },
  });

  const testMutation = useMutation({
    mutationFn: async () => {
      if (isNew) throw new Error("Save the registry first, then test.");
      return api.testConnection(arrId!, form.api_key.trim() || undefined);
    },
    onSuccess: (status) =>
      toast.success(`Connection OK${status?.version ? ` — v${status.version}` : ""}`),
    onError: (e: Error) => toast.error(`Connection failed: ${e.message}`),
  });

  if (!isNew && isLoading) {
    return <div className="text-muted-foreground py-12 text-center text-sm">Loading…</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Button asChild variant="ghost" size="sm">
            <Link to="/">
              <ArrowLeft className="size-4" />
              Back
            </Link>
          </Button>
          <div>
            <h2 className="text-xl font-semibold tracking-tight">
              {isNew ? "New registry" : (existing?.name ?? "Loading…")}
            </h2>
            <p className="text-muted-foreground mt-1 text-sm">
              {isNew
                ? "Connect a Radarr or Sonarr instance."
                : "Edit registry configuration."}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {!isNew && (
            <Button
              variant="secondary"
              disabled={testMutation.isPending}
              onClick={() => testMutation.mutate()}
            >
              <Plug className="size-4" />
              Test connection
            </Button>
          )}
          <Button disabled={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
            <Save className="size-4" />
            Save
          </Button>
        </div>
      </div>

      {/* Connection card */}
      <section className="bg-card border-border/70 rounded-2xl border p-6">
        <div className="mb-4">
          <h3 className="text-base font-semibold">Connection</h3>
          <p className="text-muted-foreground text-sm">
            Where this registry lives and how to reach it.
          </p>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="e.g. Main Radarr"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="kind">Kind</Label>
            <Select
              value={form.kind}
              onValueChange={(v) => setForm({ ...form, kind: v as Kind })}
            >
              <SelectTrigger id="kind">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="radarr">Radarr</SelectItem>
                <SelectItem value="sonarr">Sonarr</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="url">URL</Label>
            <Input
              id="url"
              type="url"
              value={form.url}
              onChange={(e) => setForm({ ...form, url: e.target.value })}
              placeholder="http://radarr:7878"
            />
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="api_key">API key</Label>
            <Input
              id="api_key"
              type="password"
              value={form.api_key}
              onChange={(e) => setForm({ ...form, api_key: e.target.value })}
              placeholder={isNew ? "" : "••••••••  (leave blank to keep current key)"}
            />
            <p className="text-muted-foreground text-xs">
              Stored encrypted at rest. Leave blank on edit to preserve the current value.
            </p>
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="root_folder_path">Root folder path</Label>
            <Input
              id="root_folder_path"
              value={form.root_folder_path}
              onChange={(e) => setForm({ ...form, root_folder_path: e.target.value })}
              placeholder={form.kind === "radarr" ? "/movies" : "/tv"}
            />
            <p className="text-muted-foreground text-xs">
              Where this *arr stores downloaded media.
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="quality_profile_id">Quality profile ID</Label>
            <Input
              id="quality_profile_id"
              type="number"
              value={form.quality_profile_id ?? ""}
              onChange={(e) =>
                setForm({
                  ...form,
                  quality_profile_id: e.target.value ? Number(e.target.value) : undefined,
                })
              }
              placeholder="e.g. 1"
            />
          </div>
          {form.kind === "sonarr" && (
            <div className="space-y-1.5">
              <Label htmlFor="language_profile_id">Language profile ID</Label>
              <Input
                id="language_profile_id"
                type="number"
                value={form.language_profile_id ?? ""}
                onChange={(e) =>
                  setForm({
                    ...form,
                    language_profile_id: e.target.value ? Number(e.target.value) : undefined,
                  })
                }
                placeholder="e.g. 1"
              />
            </div>
          )}
          <div className="flex items-center gap-3 sm:col-span-2">
            <Switch
              id="enabled"
              checked={form.enabled}
              onCheckedChange={(v) => setForm({ ...form, enabled: v })}
            />
            <Label htmlFor="enabled" className="cursor-pointer">
              Enabled
            </Label>
          </div>
        </div>
      </section>

      {/* Priority card */}
      <section className="bg-card border-border/70 rounded-2xl border p-6">
        <div className="mb-4">
          <h3 className="text-base font-semibold">Priority</h3>
          <p className="text-muted-foreground text-sm">
            Lower numbers are tried first when multiple registries match a request.
          </p>
        </div>
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="priority">Priority (integer)</Label>
          <Input
            id="priority"
            type="number"
            value={form.priority}
            onChange={(e) => setForm({ ...form, priority: Number(e.target.value) })}
          />
        </div>
      </section>

      {/* Rules card — placeholder; filled in by Task 4.4 */}
      <section className="bg-card border-border/70 rounded-2xl border p-6">
        <div>
          <h3 className="text-base font-semibold">Rules</h3>
          <p className="text-muted-foreground text-sm">Route requests to this registry when…</p>
          <p className="text-muted-foreground mt-4 text-xs italic">
            (Rule builder lands in the next commit.)
          </p>
        </div>
      </section>
    </div>
  );
}
