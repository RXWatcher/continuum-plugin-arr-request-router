import { AlertTriangle, Info, Plus, Trash2 } from "lucide-react";
import type { Combinator, Group, Kind, Op, Rule, Rules } from "@/api/types";
import {
  fieldsForKind,
  findField,
  GROUP_LABELS,
  type FieldDef,
  type FieldGroup,
} from "@/lib/fieldCatalog";
import { emptyGroup, emptyRule } from "@/lib/rules";
import { lintRules } from "@/lib/lintRules";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";

const OP_LABELS: Record<Op, string> = {
  eq: "equals",
  ne: "doesn't equal",
  in: "is one of",
  not_in: "isn't one of",
  gt: ">",
  gte: "≥",
  lt: "<",
  lte: "≤",
  between: "between",
  contains: "contains",
  starts_with: "starts with",
  regex: "matches /…/",
};

const GROUP_ORDER: FieldGroup[] = ["A", "B", "C-keywords", "C-content_rating"];

function defaultValueForField(name: string): unknown {
  const def = findField(name);
  if (!def) return "";
  switch (def.type) {
    case "bool":
      return false;
    case "number":
      return 0;
    case "date":
      return "";
    default:
      return "";
  }
}

function ensurePair(value: unknown): [number, number] {
  if (Array.isArray(value) && value.length === 2) {
    return [Number(value[0]) || 0, Number(value[1]) || 0];
  }
  return [0, 0];
}

function ensureCsv(value: unknown): string {
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "string") return value;
  return "";
}

// ── Value input — type-aware ────────────────────────────────────────────────

function ValueInput({
  fieldName,
  op,
  value,
  onChange,
}: {
  fieldName: string;
  op: Op;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const def = findField(fieldName);

  if (!def) {
    return (
      <Input
        value={typeof value === "string" ? value : ""}
        onChange={(e) => onChange(e.target.value)}
        placeholder="value"
      />
    );
  }

  if (op === "between") {
    const [lo, hi] = ensurePair(value);
    return (
      <div className="flex items-center gap-2">
        <Input
          type="number"
          className="w-24"
          value={lo}
          onChange={(e) => onChange([Number(e.target.value), hi])}
          placeholder="from"
        />
        <span className="text-muted-foreground text-xs">–</span>
        <Input
          type="number"
          className="w-24"
          value={hi}
          onChange={(e) => onChange([lo, Number(e.target.value)])}
          placeholder="to"
        />
      </div>
    );
  }

  if (op === "in" || op === "not_in") {
    return (
      <Input
        value={ensureCsv(value)}
        onChange={(e) => {
          const parts = e.target.value
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean);
          onChange(parts.length === 0 ? e.target.value : parts);
        }}
        placeholder="val1, val2, …"
      />
    );
  }

  if (def.type === "bool") {
    const bv = typeof value === "boolean" ? value : value === "true";
    return (
      <div className="flex h-9 items-center gap-2 px-1">
        <Switch checked={bv} onCheckedChange={(v) => onChange(v)} />
        <span className="text-muted-foreground text-xs">{bv ? "true" : "false"}</span>
      </div>
    );
  }

  if (def.type === "number") {
    return (
      <Input
        type="number"
        value={typeof value === "number" ? value : ""}
        onChange={(e) => onChange(e.target.value === "" ? 0 : Number(e.target.value))}
      />
    );
  }

  if (def.type === "date") {
    return (
      <Input
        type="date"
        value={typeof value === "string" ? value : ""}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }

  return (
    <Input
      value={typeof value === "string" ? value : ""}
      onChange={(e) => onChange(e.target.value)}
      placeholder={def.type === "string_array" ? "val1, val2, …" : "value"}
    />
  );
}

// ── Field picker (grouped) ──────────────────────────────────────────────────

function FieldPicker({
  value,
  available,
  onChange,
}: {
  value: string;
  available: FieldDef[];
  onChange: (name: string) => void;
}) {
  const grouped: Partial<Record<FieldGroup, FieldDef[]>> = {};
  for (const f of available) {
    if (!grouped[f.group]) grouped[f.group] = [];
    grouped[f.group]!.push(f);
  }

  return (
    <Select value={value || undefined} onValueChange={onChange}>
      <SelectTrigger>
        <SelectValue placeholder="Select field" />
      </SelectTrigger>
      <SelectContent>
        {GROUP_ORDER.filter((g) => grouped[g]?.length).map((g) => (
          <SelectGroup key={g}>
            <SelectLabel>{GROUP_LABELS[g]}</SelectLabel>
            {grouped[g]!.map((f) => (
              <SelectItem key={f.name} value={f.name}>
                {f.label}
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  );
}

// ── Single rule row ─────────────────────────────────────────────────────────

function RuleRow({
  rule,
  kind,
  onChange,
  onRemove,
}: {
  rule: Rule;
  kind: Kind;
  onChange: (next: Rule) => void;
  onRemove: () => void;
}) {
  const available = fieldsForKind(kind);
  const def = findField(rule.field);
  const allowedOps: Op[] = def ? def.ops : (["eq"] as Op[]);

  const onFieldChange = (name: string) => {
    const newDef = findField(name);
    const newOp: Op = newDef ? newDef.ops[0] : "eq";
    onChange({ field: name, op: newOp, value: defaultValueForField(name) });
  };

  const onOpChange = (next: Op) => {
    let nextValue: unknown = rule.value;
    if (next === "between" && !Array.isArray(rule.value)) {
      nextValue = [0, 0];
    } else if (
      (next === "in" || next === "not_in") &&
      !Array.isArray(rule.value) &&
      typeof rule.value !== "string"
    ) {
      nextValue = "";
    } else if (rule.op === "between" || rule.op === "in" || rule.op === "not_in") {
      nextValue = defaultValueForField(rule.field);
    }
    onChange({ ...rule, op: next, value: nextValue });
  };

  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1.4fr)_minmax(0,0.9fr)_minmax(0,1.5fr)_auto]">
      <FieldPicker value={rule.field} available={available} onChange={onFieldChange} />
      <Select value={rule.op} onValueChange={(v) => onOpChange(v as Op)}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {allowedOps.map((op) => (
            <SelectItem key={op} value={op}>
              {OP_LABELS[op]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <ValueInput
        fieldName={rule.field}
        op={rule.op}
        value={rule.value}
        onChange={(v) => onChange({ ...rule, value: v })}
      />
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Remove condition"
        onClick={onRemove}
        className="text-muted-foreground hover:text-destructive"
      >
        <Trash2 className="size-4" />
      </Button>
    </div>
  );
}

// ── Group editor ────────────────────────────────────────────────────────────

function groupSummary(g: Group): string {
  const n = g.rules.length;
  const cond = `${n} condition${n === 1 ? "" : "s"}`;
  return `${g.match.toUpperCase()} • ${cond}`;
}

function GroupEditor({
  group,
  index,
  kind,
  onChange,
  onRemove,
}: {
  group: Group;
  index: number;
  kind: Kind;
  onChange: (next: Group) => void;
  onRemove: () => void;
}) {
  const updateMatch = (m: Combinator) => onChange({ ...group, match: m });

  const addRule = () =>
    onChange({
      ...group,
      rules: [...group.rules, emptyRule(fieldsForKind(kind)[0]?.name ?? "mediaType", "eq")],
    });

  const updateRule = (ri: number, next: Rule) =>
    onChange({ ...group, rules: group.rules.map((r, i) => (i === ri ? next : r)) });

  const removeRule = (ri: number) =>
    onChange({ ...group, rules: group.rules.filter((_, i) => i !== ri) });

  return (
    <AccordionItem
      value={`group-${index}`}
      className="bg-surface border-border/70 rounded-xl border px-4"
    >
      <AccordionTrigger className="hover:no-underline">
        <div className="flex w-full items-center justify-between gap-2 pr-2">
          <span className="text-sm font-medium">Group {index + 1}</span>
          <span className="text-muted-foreground text-xs">{groupSummary(group)}</span>
        </div>
      </AccordionTrigger>
      <AccordionContent className="space-y-4 pb-4 pt-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Label className="text-muted-foreground text-xs">Combine conditions with</Label>
            <Select value={group.match} onValueChange={(v) => updateMatch(v as Combinator)}>
              <SelectTrigger className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">ALL (and)</SelectItem>
                <SelectItem value="any">ANY (or)</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={onRemove}
            className="text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="size-4" />
            Remove group
          </Button>
        </div>

        <div className="space-y-2">
          {group.rules.length === 0 && (
            <p className="text-muted-foreground py-3 text-center text-xs italic">
              No conditions yet — add one to start matching requests.
            </p>
          )}
          {group.rules.map((rule, ri) => (
            <RuleRow
              key={ri}
              rule={rule}
              kind={kind}
              onChange={(next) => updateRule(ri, next)}
              onRemove={() => removeRule(ri)}
            />
          ))}
          <div>
            <Button variant="ghost" size="sm" onClick={addRule}>
              <Plus className="size-4" />
              Add condition
            </Button>
          </div>
        </div>
      </AccordionContent>
    </AccordionItem>
  );
}

// ── Top-level rule builder ─────────────────────────────────────────────────

export default function RuleBuilder({
  value,
  onChange,
  kind,
}: {
  value: Rules;
  onChange: (next: Rules) => void;
  kind: Kind;
}) {
  const groups = value.groups;

  const updateMatch = (m: Combinator) => onChange({ ...value, match: m });
  const addGroup = () => onChange({ ...value, groups: [...groups, emptyGroup()] });
  const updateGroup = (gi: number, next: Group) =>
    onChange({ ...value, groups: groups.map((g, i) => (i === gi ? next : g)) });
  const removeGroup = (gi: number) =>
    onChange({ ...value, groups: groups.filter((_, i) => i !== gi) });

  const lints = lintRules(value, kind);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <Label className="text-muted-foreground text-xs">Match groups with</Label>
        <Select value={value.match} onValueChange={(v) => updateMatch(v as Combinator)}>
          <SelectTrigger className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">ALL (and)</SelectItem>
            <SelectItem value="any">ANY (or)</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-muted-foreground text-xs">
          {groups.length === 0
            ? "Empty rules match every request — typical for a catch-all registry."
            : `${groups.length} group${groups.length === 1 ? "" : "s"} defined.`}
        </p>
      </div>

      {lints.length > 0 && (
        <ul className="space-y-2">
          {lints.map((l, i) => {
            const isWarn = l.severity === "warn";
            const Icon = isWarn ? AlertTriangle : Info;
            return (
              <li
                key={i}
                className={`flex items-start gap-2 rounded-lg border p-2.5 text-xs ${
                  isWarn
                    ? "border-warning/30 bg-warning/10 text-warning-foreground"
                    : "border-border bg-muted/40 text-muted-foreground"
                }`}
              >
                <Icon className="mt-0.5 size-3.5 shrink-0" />
                <span>{l.message}</span>
              </li>
            );
          })}
        </ul>
      )}

      {groups.length > 0 && (
        <Accordion type="multiple" className="space-y-3">
          {groups.map((g, gi) => (
            <GroupEditor
              key={gi}
              group={g}
              index={gi}
              kind={kind}
              onChange={(next) => updateGroup(gi, next)}
              onRemove={() => removeGroup(gi)}
            />
          ))}
        </Accordion>
      )}

      <div>
        <Button variant="secondary" onClick={addGroup}>
          <Plus className="size-4" />
          Add group
        </Button>
      </div>
    </div>
  );
}
