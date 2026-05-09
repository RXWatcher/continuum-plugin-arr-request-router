import type { Status } from "../api/types";

const styles: Record<Status, string> = {
  queued:      "bg-blue-500/15 text-blue-300 border-blue-500/30",
  submitted:   "bg-blue-500/15 text-blue-300 border-blue-500/30",
  downloading: "bg-indigo-500/15 text-indigo-300 border-indigo-500/30",
  imported:    "bg-emerald-500/15 text-emerald-300 border-emerald-500/30",
  failed:      "bg-red-500/15 text-red-300 border-red-500/30",
  unrouted:    "bg-red-500/15 text-red-300 border-red-500/30",
  cancelled:   "bg-zinc-500/15 text-zinc-300 border-zinc-500/30",
};

export default function StatusPill({ status }: { status: Status }) {
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs border ${styles[status]}`}>
      {status}
    </span>
  );
}
