import { MockWorkspaceApp } from "testHelpers/entities";
import { organizeAgentApps } from "./AgentApps/AgentApps";

describe("organizeAgentApps", () => {
	test("returns one ungrouped app", () => {
		const result = organizeAgentApps([{ ...MockWorkspaceApp }]);

		expect(result).toEqual([{ apps: [MockWorkspaceApp] }]);
	});

	test("handles ordering correctly", () => {
		const bugApp = { ...MockWorkspaceApp, slug: "bug", group: "creatures" };
		const birdApp = { ...MockWorkspaceApp, slug: "bird", group: "creatures" };
		const fishApp = { ...MockWorkspaceApp, slug: "fish", group: "creatures" };
		const riderApp = { ...MockWorkspaceApp, slug: "rider" };
		const zedApp = { ...MockWorkspaceApp, slug: "zed" };
		const result = organizeAgentApps([
			bugApp,
			riderApp,
			birdApp,
			zedApp,
			fishApp,
		]);

		expect(result).toEqual([
			{ group: "creatures", apps: [bugApp, birdApp, fishApp] },
			{ apps: [riderApp] },
			{ apps: [zedApp] },
		]);
	});
});
