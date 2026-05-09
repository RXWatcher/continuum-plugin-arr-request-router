export type Kind = "radarr" | "sonarr";
export type Combinator = "all" | "any";
export type Op = "eq"|"ne"|"in"|"not_in"|"gt"|"gte"|"lt"|"lte"|"between"|"contains"|"starts_with"|"regex";

export interface Rule { field: string; op: Op; value: unknown; }
export interface Group { match: Combinator; rules: Rule[]; }
export interface Rules { match: Combinator; groups: Group[]; }

export interface RegisteredArr {
  id: number;
  name: string;
  kind: Kind;
  url: string;
  has_api_key: boolean;
  root_folder_path: string;
  quality_profile_id?: number;
  language_profile_id?: number;
  priority: number;
  enabled: boolean;
  rules: Rules;
}

export type Status = "queued"|"submitted"|"downloading"|"imported"|"failed"|"cancelled"|"unrouted";

export interface RequestRow {
  id: string;
  tmdb_id: number;
  media_type: "movie"|"tv";
  title: string;
  year: number;
  poster_url?: string;
  status: Status;
  routed_arr_id?: number;
  routed_arr_name?: string;
  external_id?: number;
  error?: string;
  match_trace?: unknown;
  submitted_at?: string;
  last_polled_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface RouteTestResult { chosen: number | null; trace: unknown; }

export interface SystemStatus {
  version?: string;
  instanceName?: string;
  appName?: string;
  branch?: string;
}
