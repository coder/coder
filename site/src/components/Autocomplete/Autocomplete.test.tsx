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
	it("renders the clear control as a native button", async () => {
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

		const clearButton = screen.getByRole("button", {
			name: "Clear selection",
		});

		expect(clearButton).toHaveAttribute("type", "button");
		expect(clearButton).not.toHaveAttribute("role");
		expect(clearButton).not.toHaveAttribute("tabindex");

		await user.click(clearButton);

		expect(onChange).toHaveBeenCalledWith(null);
	});
});
