import type { Kind, Rules } from "@/api/types";
import { findField } from "./fieldCatalog";

export type LintSeverity = "warn" | "info";

export interface RuleLint {
  severity: LintSeverity;
  message: string;
  groupIndex?: number;
  ruleIndex?: number;
}

// lintRules surfaces three classes of misconfiguration that the backend
// validator (routing.ValidateRules) does not catch:
//
//  1. Unreachable rule: in an "all" top-level match, any rule that appears
//     after an empty group is unreachable — the empty group always matches,
//     and first-match wins. (For "any" top-level, an empty group makes the
//     entire ruleset match-everything, which is its own warning.)
//
//  2. Field/kind mismatch: a movie-only field on a sonarr target (or
//     vice-versa) can never produce a true result. The backend silently
//     treats it as "no value" — operator usually intended the other kind.
//
//  3. Match-everything: an empty groups array OR a top-level "any" match
//     with any empty group means every request matches. Useful as a
//     catch-all default, but operators should know they wrote that.
//
// Lints are presentation-only; they are not enforced server-side so an
// operator who knows what they are doing can save anyway.
export function lintRules(rules: Rules, kind: Kind): RuleLint[] {
  const out: RuleLint[] = [];

  if (!rules.groups || rules.groups.length === 0) {
    out.push({
      severity: "warn",
      message:
        "No groups defined. This rule set matches every request — that may be intentional as a catch-all, but it overrides any priority-ordered rules above this target.",
    });
    return out;
  }

  const hasEmptyGroup = rules.groups.some((g) => !g.rules || g.rules.length === 0);

  if (hasEmptyGroup) {
    if (rules.match === "any") {
      out.push({
        severity: "warn",
        message:
          'Top-level match is "any" and at least one group is empty. The empty group matches everything, so the entire rule set matches every request.',
      });
    } else {
      // For "all" top-level: every group must match, so an empty group is
      // effectively a no-op (vacuously true). Flag for clarity.
      out.push({
        severity: "info",
        message:
          'Top-level match is "all" with an empty group. Empty groups are vacuously true — remove them or fill them in.',
      });
    }
  }

  rules.groups.forEach((group, gi) => {
    if (!group.rules) return;
    group.rules.forEach((rule, ri) => {
      if (!rule.field) return;
      const def = findField(rule.field);
      if (!def) {
        out.push({
          severity: "warn",
          message: `Unknown field "${rule.field}". This rule will never match.`,
          groupIndex: gi,
          ruleIndex: ri,
        });
        return;
      }
      if (def.kinds.length > 0 && !def.kinds.includes(kind)) {
        const owner = def.kinds.join(", ");
        out.push({
          severity: "warn",
          message: `Field "${def.label}" only applies to ${owner} targets. On a ${kind} target this rule never matches.`,
          groupIndex: gi,
          ruleIndex: ri,
        });
      }
    });
  });

  return out;
}
