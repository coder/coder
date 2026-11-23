import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CollapsibleSummary } from "./CollapsibleSummary";

describe("CollapsibleSummary", () => {
	it("scrolls the children into view when opened, not on mount, re-render or close", async () => {
		const user = userEvent.setup();
		const scrollIntoView = vi.mocked(HTMLElement.prototype.scrollIntoView);
		const summary = () => (
			<CollapsibleSummary label="details" defaultOpen scrollIntoViewOnOpen>
				<p>Hidden content</p>
			</CollapsibleSummary>
		);
		const { rerender } = render(summary());
		expect(scrollIntoView).not.toHaveBeenCalled();

		const toggle = screen.getByRole("button", { name: /details/i });
		await user.click(toggle);
		await user.click(toggle);
		expect(scrollIntoView).toHaveBeenCalledOnce();

		rerender(summary());
		await user.click(toggle);
		expect(scrollIntoView).toHaveBeenCalledOnce();
	});
});
