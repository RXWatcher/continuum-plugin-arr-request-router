import type { Rules, Group, Rule } from "../api/types";

export function emptyRules(): Rules { return { match: "all", groups: [] }; }
export function emptyGroup(): Group { return { match: "all", rules: [] }; }
export function emptyRule(field?: string, op?: string): Rule {
  return { field: field ?? "", op: (op as any) ?? "eq", value: "" };
}

export function normalizeRules(input: unknown): Rules {
  if (!input || typeof input !== "object") return emptyRules();
  const r = input as Partial<Rules>;
  return {
    match: r.match === "any" ? "any" : "all",
    groups: Array.isArray(r.groups)
      ? r.groups.map((g: any): Group => ({
          match: g?.match === "any" ? "any" : "all",
          rules: Array.isArray(g?.rules)
            ? g.rules.filter((x: any) => x && typeof x.field === "string" && typeof x.op === "string")
            : [],
        }))
      : [],
  };
}
