import { fireEvent, render, screen } from "@testing-library/react";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { Popover, PopoverContent, PopoverTrigger } from "./Popover";

describe("PopoverContent", () => {
	it("keeps an open dialog from cancelling wheel scrolling in the popover", async () => {
		render(
			<Dialog open>
				<DialogContent aria-describedby={undefined}>
					<DialogTitle>Dialog</DialogTitle>
					<Popover open>
						<PopoverTrigger>Open</PopoverTrigger>
						<PopoverContent>
							<button type="button">Popover action</button>
						</PopoverContent>
					</Popover>
				</DialogContent>
			</Dialog>,
		);

		// fireEvent returns false when a listener called preventDefault.
		const notCancelled = fireEvent.wheel(
			await screen.findByRole("button", { name: "Popover action" }),
			{ deltaY: 100 },
		);
		expect(notCancelled).toBe(true);
	});
});
