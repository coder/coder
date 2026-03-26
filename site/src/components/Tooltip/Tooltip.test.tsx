import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "./Tooltip";

describe("TooltipTrigger", () => {
	it("injects tabIndex={0} on non-focusable child when asChild is used", () => {
		render(
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<span>hover me</span>
					</TooltipTrigger>
					<TooltipContent>Tooltip text</TooltipContent>
				</Tooltip>
			</TooltipProvider>,
		);

		const trigger = screen.getByText("hover me");
		expect(trigger.tagName).toBe("SPAN");
		expect(trigger).toHaveAttribute("tabindex", "0");
	});

	it("respects an explicit tabIndex on the child element", () => {
		render(
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<span tabIndex={-1}>not focusable</span>
					</TooltipTrigger>
					<TooltipContent>Tooltip text</TooltipContent>
				</Tooltip>
			</TooltipProvider>,
		);

		const trigger = screen.getByText("not focusable");
		expect(trigger).toHaveAttribute("tabindex", "-1");
	});

	it("shows tooltip when non-focusable element receives keyboard focus", async () => {
		const user = userEvent.setup();

		render(
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<span>focus me</span>
					</TooltipTrigger>
					<TooltipContent>Keyboard tooltip</TooltipContent>
				</Tooltip>
			</TooltipProvider>,
		);

		const trigger = screen.getByText("focus me");
		expect(trigger).toHaveAttribute("tabindex", "0");

		await user.tab();
		expect(trigger).toHaveFocus();

		await waitFor(() => {
			expect(screen.getByRole("tooltip")).toBeInTheDocument();
		});
	});
});
