import { fireEvent, screen } from "@testing-library/react";
import type { AppFamilyName, SessionCountApp } from "#/api/typesGenerated";
import { MockDeploymentStats } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import {
	DeploymentBannerView,
	sortSessionApps,
	sumSessionFamilies,
} from "./DeploymentBannerView";

const app = (
	count: number,
	display_name: string,
	family: AppFamilyName = "unknown",
	icon?: string,
): SessionCountApp => ({ count, display_name, family, icon });

describe("sortSessionApps", () => {
	it("drops idle apps and orders by count, display name, then identifier", () => {
		const ordered = sortSessionApps({
			zulu: app(2, "zulu"),
			echo: app(1, "Alpha"),
			delta: app(1, "delta"),
			vscodium: app(1, "VSCodium"),
			codium: app(1, "VSCodium"),
			idle: app(0, "idle"),
			negative: app(-1, "negative"),
		});

		// Both VSCodium builds share a display name, so id breaks the tie.
		expect(ordered.map((sorted) => sorted.id)).toEqual([
			"zulu",
			"echo",
			"delta",
			"codium",
			"vscodium",
		]);
	});

	it("handles a deployment that reported no apps", () => {
		expect(sortSessionApps()).toEqual([]);
	});
});

describe("sumSessionFamilies", () => {
	it("totals every app into its family", () => {
		const totals = sumSessionFamilies({
			vscode: app(1, "VS Code", "vscode"),
			cursor: app(2, "Cursor", "vscode"),
			zed: app(3, "Zed", "ssh"),
			future_ide: app(4, "future_ide"),
		});

		expect(totals).toEqual(
			new Map([
				["vscode", 3],
				["ssh", 3],
				["unknown", 4],
			]),
		);
	});

	it("handles a deployment that reported no apps", () => {
		expect(sumSessionFamilies()).toEqual(new Map());
	});
});

describe("DeploymentBannerView", () => {
	// A 404 is not reproducible in a screenshot, so only a test covers this.
	it("falls back to the app name when a bundled icon fails to load", () => {
		const apps = { cursor: app(2, "Cursor", "vscode", "/icon/cursor.svg") };
		const session_count = { ...MockDeploymentStats.session_count, apps };
		render(
			<DeploymentBannerView
				stats={{ ...MockDeploymentStats, session_count }}
			/>,
		);

		fireEvent.error(screen.getByRole("img", { name: "Cursor icon" }));

		expect(screen.queryByRole("img", { name: "Cursor icon" })).toBeNull();
		screen.getByText("Cursor");
	});
});
