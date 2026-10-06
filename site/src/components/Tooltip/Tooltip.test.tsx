import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Tooltip, TooltipContent, TooltipTrigger } from "./Tooltip";

const renderPopover = (onRowClick = vi.fn()) => {
	render(
		<>
			<button type="button">Before</button>
			<div
				onClick={onRowClick}
				onKeyDown={(event) => event.key === "Enter" && onRowClick()}
			>
				<Tooltip interactive delayDuration={0}>
					<TooltipTrigger>Details</TooltipTrigger>
					<TooltipContent>
						Shared with Alice
						<a href="/one">First link</a>
						<a href="/two">Second link</a>
					</TooltipContent>
				</Tooltip>
			</div>
			<button type="button">After</button>
		</>,
	);
	return screen.getByRole("button", { name: "Details" });
};

describe("Tooltip interactive", () => {
	it("moves keyboard focus through the content in reading order", async () => {
		const user = userEvent.setup();
		const trigger = renderPopover();

		await user.tab();
		await user.tab();
		expect(trigger).toHaveFocus();
		await waitFor(() =>
			expect(trigger).toHaveAccessibleDescription(/Shared with Alice/),
		);

		await user.tab();
		expect(screen.getByRole("link", { name: "First link" })).toHaveFocus();
		await user.tab();
		expect(screen.getByRole("link", { name: "Second link" })).toHaveFocus();

		await user.tab({ shift: true });
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
		const trigger = renderPopover();

		act(() => trigger.focus());
		await user.tab();
		expect(screen.getByRole("link", { name: "First link" })).toHaveFocus();

		await user.keyboard("{Escape}");
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "false"),
		);
		expect(trigger).toHaveFocus();
	});

	it("does not move focus when it closes after a hover", async () => {
		const user = userEvent.setup();
		const trigger = renderPopover();
		const before = screen.getByRole("button", { name: "Before" });

		act(() => trigger.focus());
		await user.tab();
		await user.keyboard("{Escape}");
		act(() => before.focus());

		await user.hover(trigger);
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "true"),
		);
		await user.unhover(trigger);
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "false"),
		);
		expect(before).toHaveFocus();
	});

	it("stays open after a click until the pointer clicks it again", async () => {
		const user = userEvent.setup();
		const onRowClick = vi.fn();
		const trigger = renderPopover(onRowClick);

		await user.hover(trigger);
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "true"),
		);
		await user.click(trigger);
		await user.unhover(trigger);
		await user.hover(screen.getByRole("button", { name: "After" }));
		expect(trigger).toHaveAttribute("aria-expanded", "true");

		await user.click(screen.getByText("Shared with Alice"));
		await user.click(trigger);
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "false"),
		);
		expect(onRowClick).not.toHaveBeenCalled();
	});
});
