import { describe, expect, it } from "vitest";
import { promptFor, STAR_AFTER, SUPPORT_AFTER, type PromptState } from "./prompts";

const never = () => 1; // the support dice never hit
const always = () => 0; // the support dice always hit

const state = (s: Partial<PromptState>): PromptState => ({ plays: 0, supportSeen: false, starSeen: false, ...s });

describe("promptFor", () => {
  it.each([
    ["nothing before the support threshold", state({ plays: SUPPORT_AFTER - 1 }), always, null],
    ["support on the threshold play when never shown", state({ plays: SUPPORT_AFTER }), never, "support"],
    ["nothing between the two thresholds once support was shown", state({ plays: STAR_AFTER - 1, supportSeen: true }), never, null],
    ["star once its threshold is reached", state({ plays: STAR_AFTER, supportSeen: true }), never, "star"],
    ["star later if it was missed on the threshold play", state({ plays: STAR_AFTER + 7, supportSeen: true }), never, "star"],
    ["never star again once closed", state({ plays: STAR_AFTER + 50, supportSeen: true, starSeen: true }), never, null],
    ["support wins when both are due, star waits", state({ plays: STAR_AFTER }), never, "support"],
    ["a random support popup also makes star wait", state({ plays: STAR_AFTER, supportSeen: true }), always, "support"],
  ] as const)("%s", (_, s, random, want) => {
    expect(promptFor(s, random)).toBe(want);
  });

  it("never opens the star popup more than once over many plays", () => {
    let starSeen = false;
    let stars = 0;
    for (let plays = 1; plays <= 1000; plays++) {
      const p = promptFor({ plays, supportSeen: plays > SUPPORT_AFTER, starSeen }, Math.random);
      if (p === "star") {
        stars++;
        starSeen = true;
      }
    }
    expect(stars).toBe(1);
  });
});
