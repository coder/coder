import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { Tool } from "./Tool";

describe("Tool process_output live rendering", () => {
	// The live stream path delivers a tool call whose only identity
	// source is its own args; the process chip must render from that,
	// so a streaming poll reads as a check on the launch's process
	// without waiting for the persisted transcript pass.
	it("derives the process chip from the call args alone", () => {
		renderComponent(
			<Tool
				name="process_output"
				status="running"
				args={{ process_id: "376b2458-e318-4442-8b87-51a0f9727f0e" }}
			/>,
		);

		expect(
			screen.getByRole("img", { name: /Process 376b2458-e318/ }),
		).toHaveTextContent("376b2458");
	});

	it("omits the chip when no process_id is present", () => {
		renderComponent(<Tool name="process_output" status="running" args={{}} />);

		expect(
			screen.queryByRole("img", { name: /Process / }),
		).not.toBeInTheDocument();
	});
});
