import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { MoreHorizontal, RefreshCcw, Shuffle, XCircle } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/api/client";
import type { RequestRow, Status } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
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

type StatusFilter = "all" | Status;

const STATUS_TABS: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "queued", label: "Queued" },
  { value: "submitted", label: "Submitted" },
  { value: "downloading", label: "Downloading" },
  { value: "imported", label: "Imported" },
  { value: "failed", label: "Failed" },
  { value: "unrouted", label: "Unrouted" },
  { value: "cancelled", label: "Cancelled" },
];

const LIMIT = 25;

const STATUS_BADGE_CLASS: Record<Status, string> = {
  queued: "bg-secondary text-secondary-foreground",
  submitted: "bg-info text-info-foreground",
  downloading: "bg-info text-info-foreground",
  imported: "bg-success text-success-foreground",
  failed: "bg-destructive text-destructive-foreground",
  cancelled: "bg-muted text-muted-foreground",
  unrouted: "bg-warning text-warning-foreground",
};

function StatusBadge({ status }: { status: Status }) {
  return <Badge className={STATUS_BADGE_CLASS[status] ?? "bg-secondary"}>{status}</Badge>;
}

function relativeAge(iso?: string): string {
  if (!iso) return "—";
  const ms = Date.now() - new Date(iso).getTime();
  if (Number.isNaN(ms) || ms < 0) return "—";
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo`;
  return `${Math.floor(months / 12)}y`;
}

export default function RequestsQueuePage() {
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [page, setPage] = useState(1);
  const qc = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ["requests", statusFilter, page],
    queryFn: () =>
      api.listRequests({
        status: statusFilter === "all" ? undefined : statusFilter,
        page,
        limit: LIMIT,
      }),
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ["requests"] });

  const retry = useMutation({
    mutationFn: (id: string) => api.retryRequest(id),
    onSuccess: () => {
      invalidate();
      toast.success("Retry queued");
    },
    onError: (e: Error) => toast.error(`Retry failed: ${e.message}`),
  });

  const reroute = useMutation({
    mutationFn: (id: string) => api.reRouteRequest(id),
    onSuccess: () => {
      invalidate();
      toast.success("Re-routing");
    },
    onError: (e: Error) => toast.error(`Re-route failed: ${e.message}`),
  });

  const forceFail = useMutation({
    mutationFn: (id: string) => api.forceFailRequest(id),
    onSuccess: () => {
      invalidate();
      toast.success("Marked failed");
    },
    onError: (e: Error) => toast.error(`Force-fail failed: ${e.message}`),
  });

  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / LIMIT));

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold tracking-tight">Requests queue</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Recent requests routed by this arrouter.
        </p>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs
          value={statusFilter}
          onValueChange={(v) => {
            setStatusFilter(v as StatusFilter);
            setPage(1);
          }}
        >
          <TabsList>
            {STATUS_TABS.map((t) => (
              <TabsTrigger key={t.value} value={t.value}>
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="text-muted-foreground text-xs">
          {total} total{totalPages > 1 ? ` · page ${page}/${totalPages}` : ""}
        </div>
      </div>

      <div className="bg-card border-border/70 rounded-2xl border p-1">
        {isLoading && (
          <div className="text-muted-foreground p-8 text-center text-sm">Loading…</div>
        )}
        {error && (
          <div className="text-destructive p-8 text-center text-sm">
            {(error as Error).message}
          </div>
        )}
        {data && data.rows.length === 0 && (
          <div className="text-muted-foreground p-12 text-center text-sm">
            No requests match.
          </div>
        )}
        {data && data.rows.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Routed to</TableHead>
                <TableHead>Age</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.rows.map((r: RequestRow) => (
                <TableRow key={r.id} className="hover:bg-surface-hover">
                  <TableCell className="max-w-[280px]">
                    <div className="flex items-center gap-2">
                      {r.poster_url && (
                        <img
                          src={r.poster_url}
                          alt=""
                          className="size-8 shrink-0 rounded object-cover"
                        />
                      )}
                      <div className="min-w-0">
                        <div className="truncate font-medium">{r.title}</div>
                        {r.year ? (
                          <div className="text-muted-foreground text-xs">{r.year}</div>
                        ) : null}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{r.media_type}</Badge>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={r.status} />
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {r.routed_arr_name ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {relativeAge(r.created_at)}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm" aria-label="Actions">
                          <MoreHorizontal className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => retry.mutate(r.id)}>
                          <RefreshCcw className="size-4" />
                          Retry
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => reroute.mutate(r.id)}>
                          <Shuffle className="size-4" />
                          Re-route
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => forceFail.mutate(r.id)}
                          className="text-destructive"
                        >
                          <XCircle className="size-4" />
                          Force fail
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-end gap-2">
          <Button
            variant="ghost"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Previous
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
