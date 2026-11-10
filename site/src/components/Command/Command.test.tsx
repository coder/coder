import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Command, CommandInput, CommandItem, CommandList } from "./Command";

describe("CommandItem", () => {
	it("marks the chosen row as selected while the highlight moves", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<Command label="Fruits">
				<CommandInput aria-label="Search fruits" />
				<CommandList>
					<CommandItem aria-selected={false}>Apple</CommandItem>
					<CommandItem aria-selected>Banana</CommandItem>
					<CommandItem aria-selected={false} onSelect={onSelect}>
						Cherry
					</CommandItem>
				</CommandList>
			</Command>,
		);

		await user.click(screen.getByRole("combobox"));
		await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");
		expect(onSelect).toHaveBeenCalledWith("Cherry");
		expect(screen.getByRole("option", { selected: true })).toHaveAccessibleName(
			"Banana",
		);
	});

	it("marks the highlighted row as selected when no row has a chosen value", () => {
		render(
			<Command label="Actions">
				<CommandList>
					<CommandItem>Copy</CommandItem>
					<CommandItem>Paste</CommandItem>
				</CommandList>
			</Command>,
		);

		expect(screen.getByRole("option", { selected: true })).toHaveAccessibleName(
			"Copy",
		);
	});
});
