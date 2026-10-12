import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { genericToolLabels } from "./ToolLabel";

describe("manage_automations label", () => {
	it("names the automation that a delete removed", () => {
		const Label = genericToolLabels.manage_automations;
		expect(Label).toBeDefined();
		if (!Label) {
			return;
		}
		render(
			<Label
				name="manage_automations"
				args={{ action: "delete", automation_id: "a1" }}
				result={{ deleted: true, automation_id: "a1", name: "bb-heartbeat" }}
				isError={false}
			/>,
		);
		screen.getByText("Deleted automation bb-heartbeat");
	});
});
