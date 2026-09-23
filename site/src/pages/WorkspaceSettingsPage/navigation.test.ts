import { expect, it } from "vitest";
import { workspaceSettingsNavigation } from "./navigation";

it("returns the approved flat workspace settings order", () => {
	const sections = workspaceSettingsNavigation(
		"/@owner/workspace/settings",
		true,
	);

	expect(sections).toHaveLength(1);
	expect(sections[0]?.label).toBeUndefined();
	expect(sections[0]?.items.map((item) => item.label)).toEqual([
		"General",
		"Parameters",
		"Schedule",
		"Sharing",
	]);
});

it("applies the existing sharing permission gate", () => {
	const sections = workspaceSettingsNavigation(
		"/@owner/workspace/settings",
		false,
	);

	expect(sections[0]?.items.map((item) => item.label)).toEqual([
		"General",
		"Parameters",
		"Schedule",
	]);
});
