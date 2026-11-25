import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { WorkspaceTimingsResponse } from "./storybookData";
import { WorkspaceTimings } from "./WorkspaceTimings";

describe("WorkspaceTimings", () => {
	it("moves keyboard focus onto a script's bar", async () => {
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
		await user.click(
			screen.getByRole("button", { name: "View run startup scripts details" }),
		);

		await user.click(screen.getByRole("textbox"));
		await user.tab();

		const scriptRow = screen.getByRole("row", { name: /^Git Clone / });
		expect(within(scriptRow).getByRole("button")).toBe(document.activeElement);
	});
});
