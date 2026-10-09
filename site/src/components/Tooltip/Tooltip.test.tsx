import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Tooltip, TooltipContent, TooltipTrigger } from "./Tooltip";

const renderInteractive = (onRowClick = vi.fn()) => {
	render(
		<>
			<div onClick={onRowClick} onKeyDown={onRowClick}>
				<Tooltip interactive delayDuration={0}>
					<TooltipTrigger>Details</TooltipTrigger>
					<TooltipContent>
						Shared with Alice <a href="/one">First</a> <a href="/two">Second</a>
					</TooltipContent>
				</Tooltip>
			</div>
			<button type="button">After</button>
		</>,
	);
	return screen.getByRole("button", { name: "Details" });
};

describe("Tooltip interactive", () => {
	it("lets keyboard users tab through the content", async () => {
		const user = userEvent.setup();
		const trigger = renderInteractive();

		await user.tab();
		await waitFor(() =>
			expect(trigger).toHaveAccessibleDescription(/Shared with Alice/),
		);
		await user.tab();
		expect(screen.getByRole("link", { name: "First" })).toHaveFocus();
		await user.tab({ shift: true });
		expect(trigger).toHaveFocus();
		await user.tab();
		await user.tab();
		await user.tab();
		expect(screen.getByRole("button", { name: "After" })).toHaveFocus();
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "false"),
		);
	});

	it("closes on Escape and returns focus to the trigger", async () => {
		const user = userEvent.setup();
		const trigger = renderInteractive();

		act(() => trigger.focus());
		await user.tab();
		await user.keyboard("{Escape}");
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "false"),
		);
		expect(trigger).toHaveFocus();
	});

	it("stays open after a click without activating the row", async () => {
		const user = userEvent.setup();
		const onRowClick = vi.fn();
		const trigger = renderInteractive(onRowClick);

		await user.click(trigger);
		await user.unhover(trigger);
		await user.click(screen.getByText(/Shared with Alice/));
		expect(trigger).toHaveAttribute("aria-expanded", "true");
		expect(onRowClick).not.toHaveBeenCalled();
	});
});
