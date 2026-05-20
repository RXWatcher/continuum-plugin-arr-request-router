import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, MinusCircle, RefreshCw, XCircle } from "lucide-react";
import { api } from "@/api/client";
import type { ProbeStatus, TargetHealth } from "@/api/types";
import { Button } from "@/components/ui/button";

const PROBE_BADGE: Record<ProbeStatus, { label: string; className: string; icon: typeof CheckCircle2 }> = {
  reachable: {
    label: "reachable",
    className: "bg-success/15 text-success border border-success/30",
    icon: CheckCircle2,
  },
  unauthorized: {
    label: "unauthorized",
    className: "bg-warning/15 text-warning-foreground border border-warning/30",
    icon: AlertTriangle,
  },
  unreachable: {
    label: "unreachable",
    className: "bg-destructive/15 text-destructive border border-destructive/30",
    icon: XCircle,
  },
  skipped: {
    label: "disabled",
    className: "bg-muted text-muted-foreground border border-border",
    icon: MinusCircle,
  },
};

function relative(iso?: string): string {
  if (!iso) return "—";
  const ms = Date.now() - new Date(iso).getTime();
  if (Number.isNaN(ms) || ms < 0) return "—";
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export default function TargetsHealthPage() {
  const { data, isLoading, error, refetch, isRefetching } = useQuery({
    queryKey: ["targets-health"],
    queryFn: api.targetsHealth,
    refetchInterval: 30_000,
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">Target health</h2>
          <p className="text-muted-foreground mt-1 text-sm">
            Live SystemStatus probe and rolling 24h submission counters for every registered *arr.
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => refetch()}
          disabled={isRefetching}
        >
          <RefreshCw className={`size-4 ${isRefetching ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {isLoading && (
        <div className="text-muted-foreground rounded-2xl border bg-card p-6 text-center text-sm">
          Probing targets…
        </div>
      )}
      {error && (
        <div className="text-destructive rounded-2xl border bg-card p-6 text-center text-sm">
          {(error as Error).message}
        </div>
      )}
      {data && data.length === 0 && (
        <div className="text-muted-foreground rounded-2xl border bg-card p-6 text-center text-sm">
          No targets configured. Add a Radarr or Sonarr from the Registries tab.
        </div>
      )}
      {data && data.length > 0 && (
        <ul className="grid gap-4 sm:grid-cols-2">
          {data.map((t) => (
            <TargetCard key={t.id} t={t} />
          ))}
        </ul>
      )}
    </div>
  );
}

function TargetCard({ t }: { t: TargetHealth }) {
  const badge = PROBE_BADGE[t.probe];
  const Icon = badge.icon;
  const errorRate =
    t.submitted24h > 0 ? Math.round((t.failed24h / t.submitted24h) * 100) : 0;
  const hot = t.probe === "unreachable" || t.probe === "unauthorized" || errorRate >= 50;
  return (
    <li
      className={`rounded-2xl border bg-card p-5 space-y-4 ${
        hot ? "border-destructive/30" : "border-border/70"
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate text-base font-semibold">{t.name}</div>
          <div className="text-muted-foreground text-xs">
            <span className="capitalize">{t.kind}</span> · priority {t.priority}
            {!t.enabled && <span className="ml-1">· disabled</span>}
          </div>
          <div className="text-muted-foreground mt-0.5 truncate text-xs font-mono">
            {t.url}
          </div>
        </div>
        <span
          className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium ${badge.className}`}
        >
          <Icon className="size-3.5" />
          {badge.label}
        </span>
      </div>

      {t.probe === "reachable" && (
        <div className="text-muted-foreground text-xs">
          {t.version ? <>v{t.version} · </> : null}
          {t.probeLatencyMs}ms probe
        </div>
      )}
      {(t.probe === "unauthorized" || t.probe === "unreachable") && t.probeError && (
        <div className="text-destructive text-xs">{t.probeError}</div>
      )}

      <div className="grid grid-cols-3 gap-3 text-center text-sm">
        <div className="rounded-lg border border-border/60 p-2">
          <div className="text-muted-foreground text-xs">Sent 24h</div>
          <div className="text-base font-semibold">{t.submitted24h}</div>
        </div>
        <div className="rounded-lg border border-border/60 p-2">
          <div className="text-muted-foreground text-xs">Imported 24h</div>
          <div className="text-success text-base font-semibold">{t.imported24h}</div>
        </div>
        <div
          className={`rounded-lg border p-2 ${
            t.failed24h > 0 ? "border-destructive/30" : "border-border/60"
          }`}
        >
          <div className="text-muted-foreground text-xs">Failed 24h</div>
          <div
            className={`text-base font-semibold ${
              t.failed24h > 0 ? "text-destructive" : ""
            }`}
          >
            {t.failed24h}
          </div>
        </div>
      </div>

      <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 text-xs">
        <dt className="text-muted-foreground">Last submit</dt>
        <dd>{relative(t.lastSubmittedAt)}</dd>
        {t.lastFailureAt ? (
          <>
            <dt className="text-muted-foreground">Last failure</dt>
            <dd className="text-destructive">
              {relative(t.lastFailureAt)}
              {t.lastFailureMsg ? (
                <span className="ml-1 text-muted-foreground">— {t.lastFailureMsg}</span>
              ) : null}
            </dd>
          </>
        ) : null}
      </dl>
    </li>
  );
}
