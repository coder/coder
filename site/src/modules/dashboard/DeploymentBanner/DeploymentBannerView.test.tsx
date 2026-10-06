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
