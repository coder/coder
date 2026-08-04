import { describe, expect, it } from "vitest";
import { toolRenderers } from "./Tool";
import { genericToolLabels } from "./ToolLabel";

// process_signal's renderer delegates to GenericToolRenderer.
const genericDelegatingToolNames = new Set(["process_signal"]);

// advisor's registered renderer does not delegate; AdvisorTool renders
// ToolLabel directly with a hardcoded name instead.
const directToolLabelConsumers = new Set(["advisor"]);

// genericToolLabels holds the label for every tool name that can reach
// GenericToolRenderer. A name with a registered renderer only reaches it
// when that renderer delegates, so a name that is registered but does not
// delegate can never use its genericToolLabels entry.
describe("genericToolLabels visibility", () => {
	it("contains no arm shadowed by a non-delegating registered renderer", () => {
		const shadowed = Object.keys(genericToolLabels).filter(
			(name) =>
				!genericDelegatingToolNames.has(name) &&
				!directToolLabelConsumers.has(name) &&
				toolRenderers[name] !== undefined,
		);
		expect(shadowed).toEqual([]);
	});
});
