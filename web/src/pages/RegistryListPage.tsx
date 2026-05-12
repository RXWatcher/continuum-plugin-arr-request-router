import { Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Blocks, MoreHorizontal, Pencil, Plug, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api/client";
import type { RegisteredArr } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
