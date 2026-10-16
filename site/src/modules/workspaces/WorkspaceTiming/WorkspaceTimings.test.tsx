import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { WorkspaceTimingsResponse } from "./storybookData";
import { WorkspaceTimings } from "./WorkspaceTimings";

describe("WorkspaceTimings stages", () => {
	it("opens each stage's info tooltip from the keyboard", async () => {
		const user = userEvent.setup();
		renderComponent(
			<WorkspaceTimings
				defaultIsOpen
				provisionerTimings={WorkspaceTimingsResponse.provisioner_timings}
				agentScriptTimings={WorkspaceTimingsResponse.agent_script_timings}
				agentConnectionTimings={
					WorkspaceTimingsResponse.agent_connection_timings
				}
			/>,
		);

		const init = screen.getByRole("button", { name: "About init" });
		const plan = screen.getByRole("button", { name: "About plan" });
		act(() => init.focus());
		await user.tab();
		expect(plan).toHaveFocus();
		await waitFor(() =>
			expect(plan).toHaveAccessibleDescription(
				/^Terraform plan\s*Compare state of desired vs actual resources/,
			),
		);
	});
});
