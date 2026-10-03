import type { AppFamilyName, SessionCountApp } from "#/api/typesGenerated";
import { groupSessionApps } from "./DeploymentBannerView";

const app = (
	count: number,
	display_name: string,
	family: AppFamilyName = "unknown",
): SessionCountApp => ({ count, display_name, family });

describe("groupSessionApps", () => {
	it("groups active apps by family, ordered by display name then identifier", () => {
		const groups = groupSessionApps({
			vscodium: app(1, "VSCodium", "vscode"),
			codium: app(1, "VSCodium", "vscode"),
			cursor: app(9, "Cursor", "vscode"),
			zed: app(3, "Zed", "ssh"),
			sftp: app(2, "SFTP", "sftp"),
			future_ide: app(4, "future_ide"),
			idle: app(0, "idle", "ssh"),
			negative: app(-1, "negative", "jetbrains"),
		});

		const ids = Object.fromEntries(
			[...groups].map(([family, apps]) => [family, apps.map((a) => a.id)]),
		);
		// sftp has no slot in the banner, so it joins unknown.
		expect(ids).toEqual({
			vscode: ["cursor", "codium", "vscodium"],
			ssh: ["zed"],
			unknown: ["future_ide", "sftp"],
		});
	});

	it("handles a deployment that reported no apps", () => {
		expect(groupSessionApps()).toEqual(new Map());
	});
});
