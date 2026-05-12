import { Outlet, NavLink } from "react-router";
import { ArrowLeft, Blocks, ListChecks } from "lucide-react";
import { cn } from "@/lib/utils";

// Plain anchor (not router Link) so the browser does a full-page nav out of
// the plugin proxy back into continuum's admin section.
const backToContinuumHref = "/admin/plugins";

export default function Layout() {
  return (
    <div className="bg-background relative min-h-[100dvh] overflow-x-hidden">
      <div className="from-primary/6 pointer-events-none fixed inset-x-0 top-0 z-0 h-40 bg-gradient-to-b to-transparent blur-3xl" />

      <header className="glass-dark border-border/70 sticky top-0 z-30 mx-3 mt-3 flex items-center justify-between rounded-2xl border px-4 py-3 sm:mx-6 lg:mx-8">
        <div className="flex items-center gap-3">
          <a
            href={backToContinuumHref}
            className="text-muted-foreground hover:bg-surface-hover hover:text-foreground inline-flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium transition-colors"
            title="Back to Continuum plugins"
          >
            <ArrowLeft className="size-4" />
            <span className="hidden sm:inline">Continuum</span>
          </a>
          <span className="text-border/60" aria-hidden>
            /
          </span>
          <h1 className="text-base font-semibold tracking-tight">Arrouter</h1>
        </div>
        <nav className="flex items-center gap-1">
          <NavLink
            to="/"
            end
            className={({ isActive }) =>
              cn(
                "inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors",
                isActive
                  ? "bg-surface text-foreground"
                  : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
              )
            }
          >
            <Blocks className="size-4" />
            Registries
          </NavLink>
          <NavLink
            to="/queue"
            className={({ isActive }) =>
              cn(
                "inline-flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors",
                isActive
                  ? "bg-surface text-foreground"
                  : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
              )
            }
          >
            <ListChecks className="size-4" />
            Queue
          </NavLink>
        </nav>
      </header>

      <main
        id="main-content"
        className="relative z-10 mx-auto max-w-5xl px-4 py-6 sm:px-6 lg:px-8"
      >
        <Outlet />
      </main>
    </div>
  );
}
