import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Autocomplete } from "./Autocomplete";

interface FruitOption {
	id: string;
	name: string;
}

const fruitOptions: FruitOption[] = [
	{ id: "mango", name: "Mango" },
	{ id: "banana", name: "Banana" },
];

describe("Autocomplete", () => {
	it("renders the clear control as a non-native button", async () => {
		const onChange = vi.fn();
		const user = userEvent.setup();

		render(
			<Autocomplete
				value={fruitOptions[0]}
				onChange={onChange}
				options={fruitOptions}
				getOptionValue={(option) => option.id}
				getOptionLabel={(option) => option.name}
			/>,
		);

		const clearButton = screen.getByLabelText("Clear selection");

		expect(clearButton).toHaveAttribute("role", "button");
		expect(clearButton).toHaveAttribute("tabindex", "0");
		expect(clearButton.tagName).toBe("SPAN");

		await user.click(clearButton);

		expect(onChange).toHaveBeenCalledWith(null);
	});
});
