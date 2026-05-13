import { describe, expect, it } from "vitest";

import {
	isAgentDisplayFullyExpanded,
	isAgentDisplayOpen,
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

describe("isAgentDisplayOpen", () => {
	it("returns whether a display state shows content", () => {
		expect(isAgentDisplayOpen("collapsed")).toBe(false);
		expect(isAgentDisplayOpen("preview")).toBe(true);
		expect(isAgentDisplayOpen("expanded")).toBe(true);
	});
});

describe("isAgentDisplayFullyExpanded", () => {
	it("returns whether a display state uses a fully expanded view", () => {
		expect(isAgentDisplayFullyExpanded("expanded")).toBe(true);
		expect(isAgentDisplayFullyExpanded("preview")).toBe(false);
		expect(isAgentDisplayFullyExpanded("collapsed")).toBe(false);
	});
});
