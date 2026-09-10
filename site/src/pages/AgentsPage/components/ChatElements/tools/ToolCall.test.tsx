import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ToolCall } from "./ToolCall";

const renderCollapsible = () => {
	render(
		<ToolCall.Root
			status="completed"
			hasContent
			ariaLabel={(expanded) =>
				expanded ? "Collapse read file" : "Expand read file"
			}
		>
			<ToolCall.Header iconName="read_file" label="Read README.md" />
			<ToolCall.Content>
				<div className="mt-1.5 rounded-md border border-solid border-border-default p-3">
					File contents
				</div>
			</ToolCall.Content>
		</ToolCall.Root>,
	);
};

describe("ToolCall", () => {
	it("toggles the accessible name with the expanded state", async () => {
		const user = userEvent.setup();

		renderCollapsible();

		const headerButton = screen.getByRole("button");
		expect(headerButton).toHaveAccessibleName("Expand read file");

		await user.click(headerButton);
		expect(headerButton).toHaveAccessibleName("Collapse read file");

		await user.click(headerButton);
		expect(headerButton).toHaveAccessibleName("Expand read file");
	});
});
