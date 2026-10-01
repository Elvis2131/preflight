import { describe, it, expect } from "vitest";

// PC-104's own acceptance criterion, as a test: "no verdict/cost/fault computation exists
// client-side". Every non-test source file in this app is scanned (Vite's ?raw glob — no
// Node fs, same reason JourneyPanel.test.ts uses ?raw) for the shapes such computation
// would take. The workspace holds UI state only: it displays what the server computed
// (verdict, severed paths, utilisation, cost, delta) and never derives any of it.
const sources = import.meta.glob(["./**/*.ts", "./**/*.tsx", "!./**/*.test.ts", "!./**/*.test.tsx"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// Each rule: a name and a pattern that would only appear if this app were deriving the
// thing itself. They are deliberately about shape (function names, assignments into
// server-owned fields, arithmetic on server numbers), not about words that legitimately
// appear in displayed text.
export const RULES: Array<{ name: string; pattern: RegExp }> = [
  { name: "a function that computes a verdict/severity/blast radius/fault result", pattern: /(function\s+|const\s+)(compute|derive|calc(ulate)?)(Verdict|Severity|BlastRadius|Cascade|Severed|Survivors?|Impact|Likelihood)\b/i },
  { name: "a function that computes cost", pattern: /(function\s+|const\s+)(compute|derive|calc(ulate)?)(Cost|Price|Monthly)\w*/i },
  { name: "a function that computes flow/load/utilisation/latency", pattern: /(function\s+|const\s+)(compute|derive|calc(ulate)?)(Flow|Load|Utili[sz]ation|Latency|Reachab\w*)\b/i },
  { name: "arithmetic that rebuilds a utilisation ratio", pattern: /(offered|Offered)\w*\s*\/\s*\w*([Cc]apacity)|\w*([Cc]apacity)\w*\s*\/\s*\w*(offered|Offered)/ },
  { name: "assigning into a server-owned verdict field", pattern: /\.(verdict|severed_paths|cascade|Utilization|OfferedRPS|assurance_delta|scorecard)\s*=[^=]/ },
  { name: "a composite score (CLAUDE.md §10: per-dimension, never a single number)", pattern: /(architecture|assurance|overall)[_ ]?score/i },
];

function violations(files: Record<string, string>): string[] {
  const out: string[] = [];
  for (const [path, text] of Object.entries(files)) {
    // Comments may talk ABOUT these things (this file's own header does); only code counts.
    const code = text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:])\/\/.*$/gm, "$1");
    for (const r of RULES) {
      if (r.pattern.test(code)) out.push(`${path}: ${r.name}`);
    }
  }
  return out;
}

describe("no verdict, cost, or fault computation client-side (PC-104)", () => {
  it("scans a real set of source files", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(10);
    expect(Object.keys(sources).some((p) => p.endsWith("/App.tsx"))).toBe(true);
    expect(Object.keys(sources).some((p) => p.includes("/analyze/"))).toBe(true);
  });

  it("finds none in any source file", () => {
    expect(violations(sources)).toEqual([]);
  });

  // Negative control: the same scan MUST flag each forbidden shape when it is injected,
  // otherwise "none found" above could just mean the patterns never match anything.
  it("flags every forbidden shape when one is injected (negative control)", () => {
    const injected: Record<string, string> = {
      a: "function computeVerdict(x) { return x }",
      b: "const deriveCost = () => 1",
      c: "function computeUtilization(l) { return l.OfferedRPS / l.capacity }",
      d: "const ratio = offeredRPS / declaredCapacity",
      e: "simSummary.verdict = 'unaffected'",
      f: "const architectureScore = 87",
    };
    const flagged = violations(injected).map((v) => v.split(":")[0]);
    for (const k of Object.keys(injected)) expect(flagged, k).toContain(k);
  });

  it("does not flag the same words in a comment", () => {
    expect(violations({ x: "// function computeVerdict() — documented as forbidden\n/* offeredRPS / capacity */" })).toEqual([]);
  });
});
