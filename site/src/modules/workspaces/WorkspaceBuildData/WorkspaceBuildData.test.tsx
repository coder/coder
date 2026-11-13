import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { MockWorkspaceBuild } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	WorkspaceBuildData,
	WorkspaceBuildReasonTooltip,
} from "./WorkspaceBuildData";

const mockBuildWithReason: WorkspaceBuild = {
	...MockWorkspaceBuild,
	transition: "start",
	reason: "cli",
};

describe("WorkspaceBuildData", () => {
	it("includes the build reason in the name of the surrounding build link", () => {
		renderComponent(
			// Callers render this inside a link to the build page.
			<a href="/builds/1">
				<WorkspaceBuildData build={mockBuildWithReason} />
			</a>,
		);

		expect(screen.getByRole("link")).toHaveAccessibleName(
			/, build reason: CLI\b/,
		);
	});

	it("describes the focused build link with the build reason", async () => {
		const user = userEvent.setup();
		renderComponent(
			<WorkspaceBuildReasonTooltip build={mockBuildWithReason}>
				<a href="/builds/1">
					<WorkspaceBuildData build={mockBuildWithReason} />
				</a>
			</WorkspaceBuildReasonTooltip>,
		);

		await user.tab();

		const link = screen.getByRole("link");
		expect(link).toHaveFocus();
		await waitFor(() => expect(link).toHaveAccessibleDescription("CLI"));
	});
});
