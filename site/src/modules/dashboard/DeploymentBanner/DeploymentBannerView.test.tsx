import { fireEvent, screen } from "@testing-library/react";
import { MockDeploymentStats } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { DeploymentBannerView, sortSessionApps } from "./DeploymentBannerView";

describe("sortSessionApps", () => {
	it("drops idle apps and orders by count, display name, then identifier", () => {
		const app = (count: number, display_name: string) => ({
			count,
			display_name,
		});

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
		expect(ordered.map((app) => app.id)).toEqual([
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

describe("DeploymentBannerView", () => {
	it("falls back to the app name when a bundled icon fails to load", () => {
		render(
			<DeploymentBannerView
				stats={{
					...MockDeploymentStats,
					session_count: {
						...MockDeploymentStats.session_count,
						apps: {
							cursor: {
								count: 2,
								display_name: "Cursor",
								icon: "/icon/cursor.svg",
							},
						},
					},
				}}
			/>,
		);

		fireEvent.error(screen.getByRole("img", { name: "Cursor icon" }));

		expect(screen.queryByRole("img", { name: "Cursor icon" })).toBeNull();
		screen.getByText("Cursor");
	});
});
