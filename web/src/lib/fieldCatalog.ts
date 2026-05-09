import type { Kind, Op } from "../api/types";

export type FieldType = "string" | "number" | "bool" | "date" | "string_array";
export type FieldGroup = "A" | "B" | "C-keywords" | "C-content_rating";

export interface FieldDef {
  name: string;
  label: string;
  type: FieldType;
  group: FieldGroup;
  ops: Op[];
  kinds: Kind[]; // empty = both
  hint?: string;
}

const STRING_OPS: Op[] = ["eq", "ne", "in", "not_in", "contains", "starts_with", "regex"];
const NUMBER_OPS: Op[] = ["eq", "ne", "in", "not_in", "gt", "gte", "lt", "lte", "between"];
const BOOL_OPS: Op[] = ["eq", "ne"];
const DATE_OPS: Op[] = ["eq", "ne", "gt", "gte", "lt", "lte", "between"];
const ARRAY_OPS: Op[] = ["contains", "in", "not_in"];
const ENUM_OPS: Op[] = ["eq", "ne", "in", "not_in"];

export const FIELD_CATALOG: FieldDef[] = [
  // ── Group A (8) ──────────────────────────────────────────────────
  { name: "mediaType",          label: "Media type",           type: "string",       group: "A", ops: ENUM_OPS,   kinds: [] },
  { name: "libraryId",          label: "Library ID",           type: "string",       group: "A", ops: STRING_OPS, kinds: [] },
  { name: "year",               label: "Year",                 type: "number",       group: "A", ops: NUMBER_OPS, kinds: [] },
  { name: "decade",             label: "Decade",               type: "number",       group: "A", ops: NUMBER_OPS, kinds: [] },
  { name: "requesterUserId",    label: "Requester user ID",    type: "string",       group: "A", ops: STRING_OPS, kinds: [] },
  { name: "requesterIsAdmin",   label: "Requester is admin",   type: "bool",         group: "A", ops: BOOL_OPS,   kinds: [] },
  { name: "title",              label: "Title",                type: "string",       group: "A", ops: STRING_OPS, kinds: [] },
  { name: "tmdbId",             label: "TMDB ID",              type: "number",       group: "A", ops: NUMBER_OPS, kinds: [] },

  // ── Group B common (12) ──────────────────────────────────────────
  { name: "original_language",       label: "Original language",      type: "string",       group: "B", ops: ENUM_OPS,   kinds: [] },
  { name: "original_title",          label: "Original title",         type: "string",       group: "B", ops: STRING_OPS, kinds: [] },
  { name: "genres",                  label: "Genres",                 type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: [] },
  { name: "runtime",                 label: "Runtime (min)",          type: "number",       group: "B", ops: NUMBER_OPS, kinds: [] },
  { name: "vote_average",            label: "Vote average",           type: "number",       group: "B", ops: NUMBER_OPS, kinds: [] },
  { name: "vote_count",              label: "Vote count",             type: "number",       group: "B", ops: NUMBER_OPS, kinds: [] },
  { name: "popularity",              label: "Popularity",             type: "number",       group: "B", ops: NUMBER_OPS, kinds: [] },
  { name: "adult",                   label: "Adult",                  type: "bool",         group: "B", ops: BOOL_OPS,   kinds: [] },
  { name: "status",                  label: "Status",                 type: "string",       group: "B", ops: ENUM_OPS,   kinds: [] },
  { name: "production_companies",    label: "Production companies",   type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: [] },
  { name: "production_countries",    label: "Production countries",   type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: [] },
  { name: "spoken_languages",        label: "Spoken languages",       type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: [] },

  // ── Group B movie-only (5) ───────────────────────────────────────
  { name: "release_date",          label: "Release date",         type: "date",   group: "B", ops: DATE_OPS,   kinds: ["radarr"] },
  { name: "budget",                label: "Budget",               type: "number", group: "B", ops: NUMBER_OPS, kinds: ["radarr"] },
  { name: "revenue",               label: "Revenue",              type: "number", group: "B", ops: NUMBER_OPS, kinds: ["radarr"] },
  { name: "belongs_to_collection", label: "Belongs to collection",type: "string", group: "B", ops: STRING_OPS, kinds: ["radarr"] },
  { name: "imdb_id",               label: "IMDb ID",              type: "string", group: "B", ops: ENUM_OPS,   kinds: ["radarr"] },

  // ── Group B tv-only (9) ──────────────────────────────────────────
  { name: "networks",            label: "Networks",              type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: ["sonarr"] },
  { name: "origin_country",      label: "Origin country",        type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: ["sonarr"] },
  { name: "first_air_date",      label: "First air date",        type: "date",         group: "B", ops: DATE_OPS,   kinds: ["sonarr"] },
  { name: "last_air_date",       label: "Last air date",         type: "date",         group: "B", ops: DATE_OPS,   kinds: ["sonarr"] },
  { name: "type",                label: "Series type",           type: "string",       group: "B", ops: ENUM_OPS,   kinds: ["sonarr"] },
  { name: "in_production",       label: "In production",         type: "bool",         group: "B", ops: BOOL_OPS,   kinds: ["sonarr"] },
  { name: "number_of_seasons",   label: "Number of seasons",     type: "number",       group: "B", ops: NUMBER_OPS, kinds: ["sonarr"] },
  { name: "number_of_episodes",  label: "Number of episodes",    type: "number",       group: "B", ops: NUMBER_OPS, kinds: ["sonarr"] },
  { name: "created_by",          label: "Created by",            type: "string_array", group: "B", ops: ARRAY_OPS,  kinds: ["sonarr"] },

  // ── Group C (2) ──────────────────────────────────────────────────
  { name: "keywords",       label: "Keywords",       type: "string_array", group: "C-keywords",       ops: ARRAY_OPS, kinds: [], hint: "extra TMDB call" },
  { name: "content_rating", label: "Content rating", type: "string",       group: "C-content_rating", ops: ENUM_OPS,  kinds: [], hint: "extra TMDB call" },
];

export function fieldsForKind(kind: Kind): FieldDef[] {
  return FIELD_CATALOG.filter((f) => f.kinds.length === 0 || f.kinds.includes(kind));
}

export function findField(name: string): FieldDef | undefined {
  return FIELD_CATALOG.find((f) => f.name === name);
}

export const GROUP_LABELS: Record<FieldGroup, string> = {
  "A": "Request event",
  "B": "TMDB primary",
  "C-keywords": "TMDB keywords",
  "C-content_rating": "TMDB content rating",
};
