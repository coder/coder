import { describe, expect, it } from "vitest";

import {
	isAgentDisplayFullyExpanded,
	resolveAgentDisplayState,
} from "./displayMode";

describe("resolveAgentDisplayState", () => {
	it("resolves auto and explicit display modes", () => {
		expect(resolveAgentDisplayState(undefined, "preview")).toBe("preview");
		expect(resolveAgentDisplayState("auto", "collapsed")).toBe("collapsed");
		expect(resolveAgentDisplayState("auto", "expanded")).toBe("expanded");
		expect(resolveAgentDisplayState("always_expanded", "collapsed")).toBe(
			"expanded",
		);
		expect(resolveAgentDisplayState("always_collapsed", "expanded")).toBe(
			"collapsed",
		);
	});
});

describe("isAgentDisplayFullyExpanded", () => {
	it("returns whether a display state uses a fully expanded view", () => {
		expect(isAgentDisplayFullyExpanded("expanded")).toBe(true);
		expect(isAgentDisplayFullyExpanded("preview")).toBe(false);
		expect(isAgentDisplayFullyExpanded("collapsed")).toBe(false);
	});
});
