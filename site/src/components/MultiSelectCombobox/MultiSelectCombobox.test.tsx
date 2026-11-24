import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MultiSelectCombobox } from "./MultiSelectCombobox";

describe("MultiSelectCombobox", () => {
	it("scrolls the options into view once when opened", async () => {
		const user = userEvent.setup();
		const scrollIntoView = vi.mocked(HTMLElement.prototype.scrollIntoView);
		render(
			<MultiSelectCombobox
				commandProps={{ label: "Fruit" }}
				options={[
					{ label: "Apple", value: "apple" },
					{ label: "Banana", value: "banana" },
				]}
			/>,
		);

		await user.type(screen.getByRole("combobox", { name: "Fruit" }), "a");

		const list = screen.getByRole("listbox");
		const listScrolls = scrollIntoView.mock.contexts.filter(
			(element) => element === list,
		);
		expect(listScrolls).toHaveLength(1);
	});
});
