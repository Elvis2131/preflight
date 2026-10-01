import type { AssessResponse, DeltaKind } from "./api";

// timeline.ts is the ONE function permitted to turn a list of AssessResponse (one per
// version) into the shape this app's table renders — the same "one function,
// unit-tested, negative-controlled" discipline canvas/src/serialize.ts established.
// PC-92's own acceptance criteria, verbatim, are what this file exists to satisfy:
// "Displays per-dimension scorecard values across V1...Vn, never combined into a
// single trend line or number" and "Uses PC-19's assurance_delta output directly...
// no reimplementation." Reviewed explicitly against the no-composite-score rule
// (CLAUDE.md §10, PC-19/PC-83) before this was written, not just before merging:
// buildTimeline computes no average, no count, no ratio, no ranking of any kind. It
// looks up an already-computed status string and an already-computed DeltaKind by
// finding_id and places them in a grid position — nothing here derives a NEW fact
// about a finding that the server didn't already state.

export interface TimelineCell {
  // null means this finding did not exist in the scorecard at this version yet — a
  // real, distinct fact from any status string, never coerced into one (e.g. never
  // treated as "not_assessable", which is itself a real, different status the server
  // can and does emit).
  status: string | null;
  // The DeltaKind classifying the transition INTO this version, if this version's own
  // assurance_delta (server-computed, PC-19) contains an entry for this finding.
  // Undefined for V1 (no prior version to diff against) or when this finding wasn't
  // touched at all in that version's delta.
  deltaKind?: DeltaKind;
}

export interface TimelineRow {
  findingID: string;
  cells: TimelineCell[]; // same length and order as the input `versions` array
}

export function buildTimeline(versions: AssessResponse[]): TimelineRow[] {
  const findingIDs = new Set<string>();
  for (const v of versions) {
    for (const e of v.scorecard.entries) findingIDs.add(e.finding_id);
  }

  return Array.from(findingIDs)
    .sort()
    .map((findingID) => ({
      findingID,
      cells: versions.map((v) => {
        const entry = v.scorecard.entries.find((e) => e.finding_id === findingID);
        const deltaEntry = (v.assurance_delta ?? []).find((d) => d.finding_id === findingID);
        return {
          status: entry ? entry.status : null,
          deltaKind: deltaEntry?.kind,
        };
      }),
    }));
}
