import type { Rules, Group, Rule, Combinator, Op, Kind } from "../api/types";
import { emptyGroup, emptyRule } from "../lib/rules";
import { fieldsForKind, findField, GROUP_LABELS, type FieldGroup } from "../lib/fieldCatalog";

type Props = {
  value: Rules;
  onChange: (next: Rules) => void;
  kind: Kind;
};

// ── helpers ──────────────────────────────────────────────────────────────────

function defaultValueForField(fieldName: string): unknown {
  const def = findField(fieldName);
  if (!def) return "";
  switch (def.type) {
    case "bool":   return false;
    case "number": return 0;
    case "date":   return "";
    default:       return "";
  }
}

function ensureScalar(value: unknown): unknown {
  if (Array.isArray(value)) return "";
  return value;
}

function ensurePair(value: unknown): [unknown, unknown] {
  if (Array.isArray(value) && value.length === 2) return [value[0], value[1]];
  return [0, 0];
}

function ensureArray(value: unknown): string {
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "string") return value;
  return "";
}

// ── ValueInput ────────────────────────────────────────────────────────────────

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
  const inputCls =
    "px-2 py-1 rounded text-sm bg-[var(--surface-raised)] border border-border focus:outline-none focus:ring-1 focus:ring-primary/40";

  if (!def) {
    return (
      <input
        type="text"
        className={inputCls}
        value={typeof value === "string" ? value : ""}
        onChange={(e) => onChange(e.target.value)}
        placeholder="value"
      />
    );
  }

  // bool
  if (def.type === "bool") {
    return (
      <select
        className={inputCls}
        value={String(value)}
        onChange={(e) => onChange(e.target.value === "true")}
      >
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    );
  }

  // between
  if (op === "between") {
    const [lo, hi] = ensurePair(value);
    return (
      <span className="flex items-center gap-1">
        <input
          type="number"
          className={`${inputCls} w-24`}
          value={lo as number}
          onChange={(e) => onChange([e.target.valueAsNumber, hi])}
          placeholder="from"
        />
        <span className="text-muted-foreground text-xs">–</span>
        <input
          type="number"
          className={`${inputCls} w-24`}
          value={hi as number}
          onChange={(e) => onChange([lo, e.target.valueAsNumber])}
          placeholder="to"
        />
      </span>
    );
  }

  // in / not_in (CSV input)
  if (op === "in" || op === "not_in") {
    return (
      <input
        type="text"
        className={`${inputCls} min-w-[160px]`}
        value={ensureArray(value)}
        onChange={(e) => {
          const parts = e.target.value.split(",").map((s) => s.trim()).filter(Boolean);
          onChange(parts.length === 0 ? e.target.value : parts);
        }}
        placeholder="val1, val2, …"
      />
    );
  }

  // number
  if (def.type === "number") {
    return (
      <input
        type="number"
        className={`${inputCls} w-28`}
        value={typeof value === "number" ? value : 0}
        onChange={(e) => onChange(e.target.valueAsNumber)}
      />
    );
  }

  // date
  if (def.type === "date") {
    return (
      <input
        type="date"
        className={inputCls}
        value={typeof value === "string" ? value : ""}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }

  // string / string_array / default → text
  return (
    <input
      type="text"
      className={`${inputCls} min-w-[140px]`}
      value={typeof value === "string" ? value : ""}
      onChange={(e) => onChange(e.target.value)}
      placeholder={def.type === "string_array" ? "val1, val2, …" : "value"}
    />
  );
}

// ── RuleEditor ────────────────────────────────────────────────────────────────

function RuleEditor({
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
  const allowedOps = def ? def.ops : (["eq"] as Op[]);

  // group available fields by FieldGroup for optgroups
  const groups: Partial<Record<FieldGroup, typeof available>> = {};
  for (const f of available) {
    if (!groups[f.group]) groups[f.group] = [];
    groups[f.group]!.push(f);
  }
  const groupOrder: FieldGroup[] = ["A", "B", "C-keywords", "C-content_rating"];

  const handleFieldChange = (name: string) => {
    const newDef = findField(name);
    const newOp = newDef ? newDef.ops[0] : ("eq" as Op);
    onChange({ field: name, op: newOp, value: defaultValueForField(name) });
  };

  const handleOpChange = (op: Op) => {
    let newValue = rule.value;
    if (op === "between") {
      newValue = ensurePair(rule.value);
    } else if (op !== "in" && op !== "not_in") {
      // going away from array ops
      if (rule.op === "between" || rule.op === "in" || rule.op === "not_in") {
        newValue = defaultValueForField(rule.field);
      } else {
        newValue = ensureScalar(rule.value);
      }
    }
    onChange({ ...rule, op, value: newValue });
  };

  const selectCls =
    "px-2 py-1 rounded text-sm bg-[var(--surface-raised)] border border-border focus:outline-none focus:ring-1 focus:ring-primary/40";

  return (
    <div className="flex flex-wrap items-center gap-2 py-1.5">
      {/* Field picker */}
      <select
        className={`${selectCls} min-w-[160px]`}
        value={rule.field}
        onChange={(e) => handleFieldChange(e.target.value)}
      >
        <option value="">— pick field —</option>
        {groupOrder.map((g) => {
          const fields = groups[g];
          if (!fields || fields.length === 0) return null;
          return (
            <optgroup key={g} label={GROUP_LABELS[g]}>
              {fields.map((f) => (
                <option key={f.name} value={f.name}>
                  {f.hint ? `${f.label} (${f.hint})` : f.label}
                </option>
              ))}
            </optgroup>
          );
        })}
      </select>

      {/* Op picker */}
      <select
        className={selectCls}
        value={rule.op}
        onChange={(e) => handleOpChange(e.target.value as Op)}
        disabled={!def}
      >
        {allowedOps.map((op) => (
          <option key={op} value={op}>
            {op}
          </option>
        ))}
      </select>

      {/* Value */}
      <ValueInput
        fieldName={rule.field}
        op={rule.op}
        value={rule.value}
        onChange={(v) => onChange({ ...rule, value: v })}
      />

      {/* Remove */}
      <button
        type="button"
        onClick={onRemove}
        className="ml-1 px-2 py-1 rounded text-xs text-muted-foreground hover:text-foreground hover:bg-[var(--surface-hover)] border border-border transition-colors"
        title="Remove rule"
      >
        ×
      </button>
    </div>
  );
}

// ── GroupCard ─────────────────────────────────────────────────────────────────

function GroupCard({
  group,
  groupIndex,
  kind,
  onChange,
  onRemove,
}: {
  group: Group;
  groupIndex: number;
  kind: Kind;
  onChange: (next: Group) => void;
  onRemove: () => void;
}) {
  const addRule = () => {
    const firstField = fieldsForKind(kind)[0];
    const r = firstField
      ? emptyRule(firstField.name, firstField.ops[0])
      : emptyRule();
    onChange({ ...group, rules: [...group.rules, r] });
  };

  const updateRule = (i: number, next: Rule) => {
    const rules = group.rules.slice();
    rules[i] = next;
    onChange({ ...group, rules });
  };

  const removeRule = (i: number) => {
    onChange({ ...group, rules: group.rules.filter((_, idx) => idx !== i) });
  };

  const setMatch = (match: Combinator) => onChange({ ...group, match });

  return (
    <div className="rounded-lg border border-border bg-[var(--surface)] p-3 space-y-2">
      {/* Group header */}
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Group {groupIndex + 1}: match</span>
          <select
            className="px-2 py-0.5 rounded text-sm bg-[var(--surface-raised)] border border-border focus:outline-none"
            value={group.match}
            onChange={(e) => setMatch(e.target.value as Combinator)}
          >
            <option value="all">all</option>
            <option value="any">any</option>
          </select>
          <span className="text-muted-foreground">of</span>
        </div>
        <button
          type="button"
          onClick={onRemove}
          className="px-2.5 py-1 rounded text-xs text-muted-foreground hover:text-red-400 hover:bg-red-900/20 border border-border transition-colors"
        >
          Remove group
        </button>
      </div>

      {/* Rules */}
      <div className="pl-2 border-l-2 border-border space-y-0.5">
        {group.rules.length === 0 && (
          <p className="text-xs text-muted-foreground py-1 italic">No rules in this group.</p>
        )}
        {group.rules.map((rule, i) => (
          <RuleEditor
            key={i}
            rule={rule}
            kind={kind}
            onChange={(next) => updateRule(i, next)}
            onRemove={() => removeRule(i)}
          />
        ))}
      </div>

      <button
        type="button"
        onClick={addRule}
        className="text-xs px-2.5 py-1 rounded bg-[var(--surface-raised)] hover:bg-[var(--surface-hover)] border border-border transition-colors"
      >
        + Add rule
      </button>
    </div>
  );
}

// ── CollectionBuilder ─────────────────────────────────────────────────────────

export default function CollectionBuilder({ value, onChange, kind }: Props) {
  const setMatch = (match: Combinator) => onChange({ ...value, match });

  const addGroup = () => onChange({ ...value, groups: [...value.groups, emptyGroup()] });

  const updateGroup = (i: number, next: Group) => {
    const groups = value.groups.slice();
    groups[i] = next;
    onChange({ ...value, groups });
  };

  const removeGroup = (i: number) => {
    onChange({ ...value, groups: value.groups.filter((_, idx) => idx !== i) });
  };

  return (
    <div className="space-y-3">
      {/* Top-level combinator */}
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground">Match</span>
        <select
          className="px-2 py-0.5 rounded text-sm bg-[var(--surface)] border border-border focus:outline-none"
          value={value.match}
          onChange={(e) => setMatch(e.target.value as Combinator)}
        >
          <option value="all">all</option>
          <option value="any">any</option>
        </select>
        <span className="text-muted-foreground">of the following groups</span>
      </div>

      {/* Empty state */}
      {value.groups.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
          No rules — this *arr will match every request (catch-all).
        </div>
      )}

      {/* Groups */}
      {value.groups.map((group, i) => (
        <GroupCard
          key={i}
          group={group}
          groupIndex={i}
          kind={kind}
          onChange={(next) => updateGroup(i, next)}
          onRemove={() => removeGroup(i)}
        />
      ))}

      <button
        type="button"
        onClick={addGroup}
        className="text-sm px-3 py-1.5 rounded bg-[var(--surface)] hover:bg-[var(--surface-hover)] border border-border transition-colors"
      >
        + Add group
      </button>
    </div>
  );
}
