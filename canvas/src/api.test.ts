import { describe, it, expect } from "vitest";
import { APIError, describeSimError } from "./api";

// PC-95's own acceptance criterion, verbatim: "Canvas UI updated to branch on the new
// structured signal for at least one case (e.g. session-not-found), as proof it's
// actually usable and not just theoretically available." describeSimError is that
// branch, extracted into a pure function so it's actually testable — App.tsx's own
// real flow (assessCanvas always runs before simulateNodeLoss) never triggers
// session_not_found through normal clicking, since assessCanvas creates the session
// first; this test exercises the branch directly rather than requiring a contrived
// live scenario just to reach it.

describe("describeSimError", () => {
  it("gives session_not_found a distinct, actionable message", () => {
    const err = new APIError(404, "session_not_found", "server: no session x found");
    const msg = describeSimError(err);
    expect(msg).toContain("no longer exists on the server");
    expect(msg).toContain("session_not_found");
  });

  it("falls back to the plain message for any OTHER APIError code", () => {
    const err = new APIError(404, "version_not_found", "server: no version 5 found");
    expect(describeSimError(err)).toBe("server: no version 5 found");
  });

  it("falls back to the plain message for a non-APIError Error", () => {
    expect(describeSimError(new Error("network failure"))).toBe("network failure");
  });

  it("stringifies a non-Error thrown value", () => {
    expect(describeSimError("just a string")).toBe("just a string");
  });
});
