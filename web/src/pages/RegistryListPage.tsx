import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Blocks, MoreHorizontal, Pencil, Plug, Save, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api/client";
import type { AppConfig, RegisteredArr } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export default function RegistryListPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["arrs"],
    queryFn: api.listArrs,
  });
  const configQuery = useQuery({
    queryKey: ["config"],
    queryFn: api.config,
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">Registered *arrs</h2>
          <p className="text-muted-foreground mt-1 text-sm">
            Radarr and Sonarr instances that requests can route to.
          </p>
        </div>
        <Button asChild>
          <Link to="/registry/new">
            <Plus className="size-4" />
            Add registry
          </Link>
        </Button>
      </div>

      {configQuery.data ? (
        <ConfigPanel config={configQuery.data} />
      ) : configQuery.isLoading ? (
        <div className="text-muted-foreground bg-card border-border/70 rounded-2xl border p-6 text-sm">
          Loading settings…
        </div>
      ) : configQuery.error ? (
        <div className="text-destructive bg-card border-border/70 rounded-2xl border p-6 text-sm">
          Failed to load settings: {(configQuery.error as Error).message}
        </div>
      ) : null}

      <div className="bg-card border-border/70 rounded-2xl border p-1">
        {isLoading && (
          <div className="text-muted-foreground p-8 text-center text-sm">Loading…</div>
        )}
        {error && (
          <div className="text-destructive p-8 text-center text-sm">
            Failed to load: {(error as Error).message}
          </div>
        )}
        {data && data.length === 0 && (
          <div className="flex flex-col items-center gap-3 px-6 py-12 text-center">
            <Blocks className="text-muted-foreground size-12 opacity-40" />
            <div className="text-base font-medium">No registries yet</div>
            <p className="text-muted-foreground max-w-sm text-sm">
              Add your first Radarr or Sonarr to start routing requests.
            </p>
            <Button asChild className="mt-2">
              <Link to="/registry/new">
                <Plus className="size-4" />
                Add registry
              </Link>
            </Button>
          </div>
        )}
        {data && data.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-20">Priority</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>URL</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Rule groups</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <RegistryRow key={r.id} arr={r} />
              ))}
            </TableBody>
          </Table>
        )}
      </div>
    </div>
  );
}

function ConfigPanel({ config }: { config: AppConfig }) {
  const qc = useQueryClient();
  const [form, setForm] = useState<AppConfig>(config);

  useEffect(() => {
    setForm(config);
  }, [config]);

  const saveMutation = useMutation({
    mutationFn: api.updateConfig,
    onSuccess: (updated) => {
      setForm(updated);
      qc.setQueryData(["config"], updated);
      toast.success("Settings saved");
    },
    onError: (e: Error) => toast.error(`Save failed: ${e.message}`),
  });

  const setText = (key: keyof AppConfig) => (value: string) => {
    setForm((current) => ({ ...current, [key]: value }));
  };
  const setNumber = (key: keyof AppConfig) => (value: string) => {
    setForm((current) => ({ ...current, [key]: value === "" ? 0 : Number(value) }));
  };

  return (
    <section className="bg-card border-border/70 rounded-2xl border p-6">
      <div className="mb-5 flex items-center justify-between gap-3">
        <div>
          <h3 className="text-base font-semibold">Router settings</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            TMDB enrichment, polling, and stored credential encryption.
          </p>
        </div>
        <Button
          size="sm"
          disabled={saveMutation.isPending}
          onClick={() => saveMutation.mutate(form)}
        >
          <Save className="size-4" />
          Save
        </Button>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="tmdb_api_key">TMDB API key</Label>
          <Input
            id="tmdb_api_key"
            type="password"
            value={form.tmdb_api_key}
            onChange={(e) => setText("tmdb_api_key")(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="tmdb_language">TMDB language</Label>
          <Input
            id="tmdb_language"
            value={form.tmdb_language}
            onChange={(e) => setText("tmdb_language")(e.target.value)}
            placeholder="en-US"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="poll_interval_seconds">Poll interval seconds</Label>
          <Input
            id="poll_interval_seconds"
            type="number"
            min={10}
            max={600}
            value={form.poll_interval_seconds}
            onChange={(e) => setNumber("poll_interval_seconds")(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="stale_after_hours">Stale after hours</Label>
          <Input
            id="stale_after_hours"
            type="number"
            min={1}
            value={form.stale_after_hours}
            onChange={(e) => setNumber("stale_after_hours")(e.target.value)}
          />
        </div>
        <div className="space-y-1.5 sm:col-span-2">
          <Label htmlFor="secret_key">Secret encryption key</Label>
          <Input
            id="secret_key"
            type="password"
            value={form.secret_key}
            onChange={(e) => setText("secret_key")(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            Changing this key prevents existing registered API keys from decrypting.
          </p>
        </div>
      </div>
    </section>
  );
}

function RegistryRow({ arr }: { arr: RegisteredArr }) {
  const qc = useQueryClient();

  const testMutation = useMutation({
    mutationFn: () => api.testConnection(arr.id),
    onSuccess: (status) => toast.success(`Connection OK${status?.version ? ` — v${status.version}` : ""}`),
    onError: (e: Error) => toast.error(`Connection failed: ${e.message}`),
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteArr(arr.id),
    onSuccess: () => {
      toast.success(`Deleted "${arr.name}"`);
      qc.invalidateQueries({ queryKey: ["arrs"] });
    },
    onError: (e: Error) => toast.error(`Delete failed: ${e.message}`),
  });

  const ruleGroups = arr.rules?.groups?.length ?? 0;

  return (
    <TableRow className="hover:bg-surface-hover">
      <TableCell className="font-mono tabular-nums">{arr.priority}</TableCell>
      <TableCell>
        <Link to={`/registry/${arr.id}`} className="font-medium hover:underline">
          {arr.name}
        </Link>
      </TableCell>
      <TableCell>
        <Badge variant="secondary">{arr.kind}</Badge>
      </TableCell>
      <TableCell className="text-muted-foreground font-mono text-xs">{arr.url}</TableCell>
      <TableCell>
        <Badge variant={arr.enabled ? "default" : "secondary"}>
          {arr.enabled ? "Enabled" : "Disabled"}
        </Badge>
      </TableCell>
      <TableCell className="text-right tabular-nums">{ruleGroups}</TableCell>
      <TableCell>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Actions">
              <MoreHorizontal className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem asChild>
              <Link to={`/registry/${arr.id}`}>
                <Pencil className="size-4" />
                Edit
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => testMutation.mutate()}
              disabled={testMutation.isPending}
            >
              <Plug className="size-4" />
              Test connection
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => {
                if (!confirm(`Delete registry "${arr.name}"?`)) return;
                deleteMutation.mutate();
              }}
              className="text-destructive"
              disabled={deleteMutation.isPending}
            >
              <Trash2 className="size-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  );
}
