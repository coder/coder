import { describe, expect, it } from "vitest";
import { toolRenderers } from "./Tool";
import { toolIcons } from "./ToolIcon";

// ReadSkillFileRenderer renders ReadSkillTool, which passes the
// hardcoded iconName "read_skill" for read_skill_file calls.
const aliasedIconNames = new Set(["read_skill_file"]);

describe("toolIcons coverage", () => {
	// A registered tool name without a table entry silently degrades to
	// WrenchIcon, unless its renderer passes an aliased fixed iconName.
	it("resolves an icon for every registered renderer", () => {
		const missing = Object.keys(toolRenderers).filter(
			(name) => !aliasedIconNames.has(name) && toolIcons[name] === undefined,
		);
		expect(missing).toEqual([]);
	});
});
