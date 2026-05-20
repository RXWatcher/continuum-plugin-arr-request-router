import { describe, it, expect } from "vitest";
import { lintRules } from "./lintRules";
import type { Rules } from "@/api/types";

describe("lintRules", () => {
  it("warns when there are no groups", () => {
    const lints = lintRules({ match: "all", groups: [] }, "radarr");
    expect(lints).toHaveLength(1);
    expect(lints[0].severity).toBe("warn");
    expect(lints[0].message).toMatch(/matches every request/);
  });

  it('warns about empty groups in "any" mode (match-everything)', () => {
    const rules: Rules = {
      match: "any",
      groups: [
        { match: "all", rules: [{ field: "year", op: "gte", value: 2020 }] },
        { match: "all", rules: [] },
      ],
    };
    const lints = lintRules(rules, "radarr");
    expect(lints.find((l) => l.message.includes("matches every request"))).toBeTruthy();
  });

  it('info-level note about empty groups in "all" mode', () => {
    const rules: Rules = {
      match: "all",
      groups: [
        { match: "all", rules: [{ field: "year", op: "gte", value: 2020 }] },
        { match: "all", rules: [] },
      ],
    };
    const lints = lintRules(rules, "radarr");
    const info = lints.find((l) => l.severity === "info");
    expect(info?.message).toMatch(/vacuously true/);
  });

  it("warns when a radarr-only field is used on a sonarr target", () => {
    const rules: Rules = {
      match: "all",
      groups: [
        {
          match: "all",
          rules: [{ field: "release_date", op: "gte", value: "2020-01-01" }],
        },
      ],
    };
    const lints = lintRules(rules, "sonarr");
    expect(
      lints.find((l) => l.message.includes("only applies to radarr")),
    ).toBeTruthy();
  });

  it("warns when a sonarr-only field is used on a radarr target", () => {
    const rules: Rules = {
      match: "all",
      groups: [
        {
          match: "all",
          rules: [{ field: "networks", op: "contains", value: "HBO" }],
        },
      ],
    };
    const lints = lintRules(rules, "radarr");
    expect(
      lints.find((l) => l.message.includes("only applies to sonarr")),
    ).toBeTruthy();
  });

  it("flags unknown fields as never-matching", () => {
    const rules: Rules = {
      match: "all",
      groups: [
        { match: "all", rules: [{ field: "wat", op: "eq", value: 1 }] },
      ],
    };
    const lints = lintRules(rules, "radarr");
    expect(lints[0].message).toMatch(/Unknown field/);
  });

  it("returns no lints for a well-formed cross-kind ruleset", () => {
    const rules: Rules = {
      match: "all",
      groups: [
        {
          match: "all",
          rules: [
            { field: "year", op: "gte", value: 2020 },
            { field: "vote_average", op: "gte", value: 6 },
          ],
        },
      ],
    };
    expect(lintRules(rules, "radarr")).toHaveLength(0);
    expect(lintRules(rules, "sonarr")).toHaveLength(0);
  });
});
