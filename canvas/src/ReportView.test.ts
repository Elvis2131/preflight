import { describe, it, expect } from "vitest";
// Vite's own `?raw` import suffix loads the file as a plain string at build time —
// no Node fs/path/url APIs needed, which matters here specifically: tsconfig.app.json
// only declares browser types (["vite/client"], no "node"), so a Node-API import in
// any file under src/ fails `tsc -b` (npm run build) even though `tsc --noEmit`
// alone doesn't catch it — found by actually running the full build, not just the
// type checker in isolation.
import reportViewSource from "./ReportView.tsx?raw";

// PC-123's own acceptance criterion, verbatim: "Version comparison shows
// server-provided deltas only; test/grep confirms no client-side diff logic." Static
// source inspection, the same "prove it structurally, not just by reading the code
// once" discipline this whole project applies to its Go side (e.g. core/sizing_test.go's
// own go/ast walk proving Sizing is never read by the capacity engine).

const source = reportViewSource;

describe("ReportView: no client-side diff logic", () => {
  it("ReportSummary takes exactly one Report value, never two", () => {
    // A diffing function would need a signature like
    // `function diff(a: Report, b: Report)` — no such signature exists; every
    // component in this file takes at most one `report: Report` prop.
    const twoReportParams = /\(\s*\w+\s*:\s*Report\s*,\s*\w+\s*:\s*Report\s*\)/;
    expect(twoReportParams.test(source)).toBe(false);
  });

  it("reportA and reportB never appear on the same line outside the two known, harmless render call sites", () => {
    const allowedLines = [
      "{reportA && reportB && (", // the JSX condition gating the side-by-side layout
      "{reportA && !reportB && <ReportSummary report={reportA} onRepriced={reprice} />}", // single-report view
    ];
    const lines = source.split("\n");
    const offendingLines = lines.filter((line) => {
      if (!line.includes("reportA") || !line.includes("reportB")) return false;
      return !allowedLines.some((allowed) => line.trim() === allowed);
    });
    expect(offendingLines).toEqual([]);
  });

  it("does not import or define any function with 'diff' in its name", () => {
    // A real diff computation would need some function to do it in — grepping for
    // the word itself catches an accidental reintroduction long before it grows into
    // real logic.
    expect(/\bfunction\s+\w*[Dd]iff\w*\s*\(/.test(source)).toBe(false);
    expect(/\bconst\s+\w*[Dd]iff\w*\s*=/.test(source)).toBe(false);
  });
});
