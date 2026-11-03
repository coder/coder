import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Table, TableBody } from "#/components/Table/Table";
import { TableEmptyState } from "./TableEmptyState";

describe("TableEmptyState", () => {
	it("calls onResetFilters when Reset filters is clicked", async () => {
		const onResetFilters = vi.fn();
		render(
			<Table>
				<TableBody>
					<TableEmptyState
						variant="search"
						message="No users match your search"
						onResetFilters={onResetFilters}
					/>
				</TableBody>
			</Table>,
		);

		await userEvent.click(
			screen.getByRole("button", { name: "Reset filters" }),
		);

		expect(onResetFilters).toHaveBeenCalledOnce();
	});
});
