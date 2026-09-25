import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { BoardHeader } from "./BoardHeader";

const renderHeader = (effortFilter: string | null) => {
	const onFilterEffort = vi.fn();
	const onRenameEffort = vi.fn();
	renderComponent(
		<BoardHeader
			chatCount={3}
			cardCount={2}
			visibleCount={undefined}
			search=""
			onSearchChange={vi.fn()}
			onExit={vi.fn()}
			onBoardAssistant={vi.fn()}
			effortCounts={[
				{ name: "Q3", count: 2 },
				{ name: "This week", count: 1 },
			]}
			effortFilter={effortFilter}
			onFilterEffort={onFilterEffort}
			onRenameEffort={onRenameEffort}
		/>,
	);
	return { onFilterEffort, onRenameEffort };
};

describe("BoardHeader", () => {
	it("selects an effort from the menu and clears it with All", async () => {
		const user = userEvent.setup();
		const { onFilterEffort } = renderHeader(null);

		await user.click(screen.getByRole("button", { name: "Efforts" }));
		await user.click(
			await screen.findByRole("menuitemradio", { name: /This week/ }),
		);
		expect(onFilterEffort).toHaveBeenCalledWith("This week");

		await user.click(screen.getByRole("button", { name: "Efforts" }));
		await user.click(await screen.findByRole("menuitemradio", { name: /All/ }));
		expect(onFilterEffort).toHaveBeenLastCalledWith(null);
	});

	it("renames the selected effort in place and cancels on Escape", async () => {
		const user = userEvent.setup();
		const { onRenameEffort } = renderHeader("Q3");

		await user.click(screen.getByRole("button", { name: "Q3" }));
		await user.click(
			await screen.findByRole("menuitem", { name: "Rename effort" }),
		);
		const field = await screen.findByRole("textbox", { name: "Effort name" });
		await user.clear(field);
		await user.type(field, "Q4{Enter}");
		expect(onRenameEffort).toHaveBeenCalledWith("Q3", "Q4");

		await user.click(screen.getByRole("button", { name: "Q3" }));
		await user.click(
			await screen.findByRole("menuitem", { name: "Rename effort" }),
		);
		await user.type(
			await screen.findByRole("textbox", { name: "Effort name" }),
			"{Escape}",
		);
		expect(onRenameEffort).toHaveBeenCalledTimes(1);
	});
});
