import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CollapsibleSummary } from "./CollapsibleSummary";

describe("CollapsibleSummary", () => {
	it("scrolls the children into view when opened", async () => {
		const user = userEvent.setup();
		const scrollIntoView = vi.mocked(HTMLElement.prototype.scrollIntoView);
		render(
			<CollapsibleSummary label="details" scrollIntoViewOnOpen>
				<p>Hidden content</p>
			</CollapsibleSummary>,
		);

		await user.click(screen.getByRole("button", { name: /details/i }));
		expect(scrollIntoView).toHaveBeenCalledOnce();
	});
});
