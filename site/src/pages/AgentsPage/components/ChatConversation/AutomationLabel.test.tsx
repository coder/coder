import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { AutomationLabel } from "./AutomationLabel";

const automationId = "7f1c2b9e-4d3a-4c1f-9b2e-5a6d7e8f9a0b";
const inputId = "0b6c4e2a-1f3d-4b5c-8a9e-7d6c5b4a3f2e";

describe("AutomationLabel", () => {
	it.each(["card", "badge"] as const)(
		"shows the full IDs when the %s label gets keyboard focus",
		async (variant) => {
			const user = userEvent.setup();
			render(
				<TooltipProvider delayDuration={0}>
					<AutomationLabel
						variant={variant}
						automationId={automationId}
						inputId={inputId}
						reference={{
							id: automationId,
							name: "CI heartbeat",
							kind: "schedule",
						}}
						nameStatus="settled"
					/>
				</TooltipProvider>,
			);

			const label = screen.getByRole("note", {
				name: "Automation run · CI heartbeat (schedule)",
			});
			// The visible text is one contiguous run, so it copies as one line.
			expect(label.textContent).toBe(
				"Automation run · CI heartbeat (schedule)",
			);

			await user.tab();
			expect(label).toHaveFocus();
			const tooltip = await screen.findByRole("tooltip");
			expect(tooltip).toHaveTextContent(`Automation ID: ${automationId}`);
			expect(tooltip).toHaveTextContent(`Input ID: ${inputId}`);
		},
	);

	it("keeps the UUID out of the label when the references request fails", async () => {
		const user = userEvent.setup();
		render(
			<TooltipProvider delayDuration={0}>
				<AutomationLabel
					variant="card"
					automationId={automationId}
					inputId={inputId}
					nameStatus="error"
				/>
			</TooltipProvider>,
		);

		const label = screen.getByRole("note", { name: "Automation run" });
		expect(label).not.toHaveTextContent(automationId);

		await user.tab();
		const tooltip = await screen.findByRole("tooltip");
		expect(tooltip).toHaveTextContent("Could not load the automation name.");
		expect(tooltip).toHaveTextContent(`Automation ID: ${automationId}`);
	});
});
