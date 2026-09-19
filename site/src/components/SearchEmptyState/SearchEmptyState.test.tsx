import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SearchEmptyState } from "./SearchEmptyState";

describe("SearchEmptyState", () => {
	it("calls onClearFilters when the reset button is clicked", async () => {
		const onClearFilters = vi.fn();
		render(
			<SearchEmptyState
				message="No users match your search"
				onClearFilters={onClearFilters}
			/>,
		);

		await userEvent.click(
			screen.getByRole("button", { name: /reset filters/i }),
		);

		expect(onClearFilters).toHaveBeenCalledTimes(1);
	});
});
