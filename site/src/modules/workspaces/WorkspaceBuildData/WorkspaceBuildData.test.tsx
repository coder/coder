import { screen } from "@testing-library/react";
import { MockWorkspaceBuild } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { WorkspaceBuildData } from "./WorkspaceBuildData";

describe("WorkspaceBuildData", () => {
	it("includes the build reason in the name of the surrounding build link", () => {
		renderComponent(
			// Callers render this inside a link to the build page.
			<a href="/builds/1">
				<WorkspaceBuildData
					build={{ ...MockWorkspaceBuild, transition: "start", reason: "cli" }}
				/>
			</a>,
		);

		expect(screen.getByRole("link")).toHaveAccessibleName(
			/, build reason: CLI\b/,
		);
	});
});
