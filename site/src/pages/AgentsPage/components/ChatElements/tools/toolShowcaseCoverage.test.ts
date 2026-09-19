import { describe, expect, it } from "vitest";
import { toolRenderers } from "./Tool";
import { allToolShowcaseItems } from "./toolShowcaseFixtures";

// A registered renderer without a showcase fixture would silently drop out
// of the policy-badge grid story, so every renderer must stay listed.
describe("tool showcase coverage", () => {
	it("covers every registered renderer with a showcase fixture", () => {
		const covered = new Set(allToolShowcaseItems.map((tool) => tool.name));
		const missing = Object.keys(toolRenderers).filter(
			(name) => !covered.has(name),
		);
		expect(missing).toEqual([]);
	});
});
