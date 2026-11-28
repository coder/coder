import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceSharingIndicator } from "./WorkspaceSharingIndicator";

describe("WorkspaceSharingIndicator", () => {
	it("lets keyboard users reach the permissions list", async () => {
		const user = userEvent.setup();
		render(
			<WorkspaceSharingIndicator
				sharedWith={[
					{ id: "alice", actor_type: "user", name: "alice", roles: ["use"] },
				]}
				settingsPath="/@owner/my-workspace/settings/sharing"
			/>,
		);

		await user.tab();
		await screen.findByRole("dialog");
		await user.tab();

		expect(
			screen.getByRole("list", { name: "Workspace permissions" }),
		).toHaveFocus();
	});
});
