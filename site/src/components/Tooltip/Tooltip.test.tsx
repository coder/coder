import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { renderComponent } from "#/testHelpers/renderHelpers";
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

describe("Tooltip interactive with text only", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	const renderTextOnly = ({ withAfter = true } = {}) => {
		render(
			<>
				<Tooltip interactive delayDuration={0}>
					<TooltipTrigger>Details</TooltipTrigger>
					<TooltipContent>Long details</TooltipContent>
				</Tooltip>
				{withAfter && <button type="button">After</button>}
			</>,
		);
		return screen.getByRole("button", { name: "Details" });
	};

	// jsdom has no layout, so report the content as taller than its box.
	const mockScrollingContent = () =>
		vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(500);

	it("moves Tab into content that scrolls, then out past the trigger", async () => {
		mockScrollingContent();
		const user = userEvent.setup();
		renderTextOnly();

		await user.tab();
		await user.tab();
		expect(screen.getByRole("dialog")).toHaveFocus();
		await user.tab();
		expect(screen.getByRole("button", { name: "After" })).toHaveFocus();
	});

	it("lets Tab leave content that scrolls when nothing follows the trigger", async () => {
		mockScrollingContent();
		const user = userEvent.setup();
		const trigger = renderTextOnly({ withAfter: false });

		await user.tab();
		await user.tab();
		expect(screen.getByRole("dialog")).toHaveFocus();
		await user.tab();
		expect(document.body).toHaveFocus();
		await waitFor(() =>
			expect(trigger).toHaveAttribute("aria-expanded", "false"),
		);
	});

	it("does not add a Tab stop for content that fits", async () => {
		const user = userEvent.setup();
		renderTextOnly({ withAfter: false });

		await user.tab();
		const dialog = await screen.findByRole("dialog");
		await user.tab();
		expect(dialog).not.toHaveFocus();
	});
});

describe("Tooltip delay", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it.each([
		{ delayDuration: undefined, expectedDelay: 100 },
		{ delayDuration: 150, expectedDelay: 150 },
		{ delayDuration: 300, expectedDelay: 300 },
	])(
		"opens after $expectedDelay ms",
		async ({ delayDuration, expectedDelay }) => {
			const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
			const onOpenChange = vi.fn();
			renderComponent(
				<Tooltip delayDuration={delayDuration} onOpenChange={onOpenChange}>
					<TooltipTrigger>Details</TooltipTrigger>
					<TooltipContent>More information</TooltipContent>
				</Tooltip>,
			);

			await user.hover(screen.getByRole("button", { name: "Details" }));
			act(() => vi.advanceTimersByTime(expectedDelay - 1));
			expect(onOpenChange).not.toHaveBeenCalled();
			act(() => vi.advanceTimersByTime(1));
			expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(true);
		},
	);
});

describe("TooltipContent", () => {
	it.each([
		{ interactive: false, target: () => screen.findByRole("tooltip") },
		{
			interactive: true,
			target: () => screen.findByRole("link", { name: "Docs" }),
		},
	])(
		"keeps an open dialog from cancelling wheel scrolling (interactive: $interactive)",
		async ({ interactive, target }) => {
			renderComponent(
				<Dialog open>
					<DialogContent aria-describedby={undefined}>
						<DialogTitle>Dialog</DialogTitle>
						<Tooltip
							open={interactive ? undefined : true}
							interactive={interactive}
						>
							<TooltipTrigger>Details</TooltipTrigger>
							<TooltipContent>
								Long content <a href="/docs">Docs</a>
							</TooltipContent>
						</Tooltip>
					</DialogContent>
				</Dialog>,
			);
			if (interactive) {
				await userEvent.click(screen.getByRole("button", { name: "Details" }));
			}

			// fireEvent returns false when a listener called preventDefault.
			expect(fireEvent.wheel(await target(), { deltaY: 100 })).toBe(true);
		},
	);
});
