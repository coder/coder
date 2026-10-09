import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { AppFamilyName, SessionCountApp } from "#/api/typesGenerated";
import { MockDeploymentStats } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { DeploymentBannerView, groupSessionApps } from "./DeploymentBannerView";

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

describe("DeploymentBannerView", () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it("updates the last aggregated time as the refresh countdown ticks", () => {
		vi.useFakeTimers();
		const now = new Date("2023-03-06T19:13:35.000Z");
		vi.setSystemTime(now);
		const stats = {
			...MockDeploymentStats,
			collected_at: new Date(now.getTime() - 40_000).toISOString(),
			next_update_at: new Date(now.getTime() + 60_000).toISOString(),
		};
		render(<DeploymentBannerView stats={stats} fetchStats={vi.fn()} />);

		expect(
			screen.getByRole("button", { name: "a few seconds ago" }),
		).toBeInTheDocument();

		act(() => {
			vi.advanceTimersByTime(10_000);
		});

		expect(
			screen.getByRole("button", { name: "a minute ago" }),
		).toBeInTheDocument();
	});
	it("exposes stat tooltips to keyboard and screen reader users", async () => {
		const user = userEvent.setup();
		render(<DeploymentBannerView stats={MockDeploymentStats} />);

		const trigger = await screen.findByRole("button", {
			name: "Deployment status",
		});
		await user.tab();
		expect(trigger).toHaveFocus();
		await waitFor(() =>
			expect(trigger).toHaveAccessibleDescription(
				"Status of your Coder deployment. Only visible for admins!",
			),
		);

		const transmission = screen.getByRole("button", { name: "Transmission" });
		act(() => transmission.focus());
		await waitFor(() =>
			expect(transmission).toHaveAccessibleDescription(
				/^Activity in the last ~\d+ minutes$/,
			),
		);
	});
});
