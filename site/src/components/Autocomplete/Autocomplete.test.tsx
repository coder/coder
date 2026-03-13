import { renderComponent } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { Autocomplete } from "./Autocomplete";

interface FruitOption {
	id: string;
	name: string;
}

const fruitOptions: FruitOption[] = [
	{ id: "1", name: "Mango" },
	{ id: "2", name: "Banana" },
	{ id: "3", name: "Pineapple" },
];

const renderAutocomplete = () => {
	const onChange = vi.fn<(value: FruitOption | null) => void>();

	const TestComponent = () => {
		const [value, setValue] = useState<FruitOption | null>(fruitOptions[0]);

		const handleChange = (newValue: FruitOption | null) => {
			onChange(newValue);
			setValue(newValue);
		};

		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={handleChange}
					options={fruitOptions}
					getOptionValue={(option) => option.id}
					getOptionLabel={(option) => option.name}
					placeholder="Select a fruit"
				/>
			</div>
		);
	};

	renderComponent(<TestComponent />);

	return { onChange };
};

describe(Autocomplete.name, () => {
	it("does not render nested interactive elements in the trigger", () => {
		renderAutocomplete();

		const trigger = screen.getByRole("button", { name: /mango/i });
		const clearButton = screen.getByRole("button", {
			name: /clear selection/i,
		});

		expect(trigger).not.toContainElement(clearButton);
		expect(
			trigger.querySelector(
				"button, a, input, select, textarea, [role='button'], [tabindex]:not([tabindex='-1'])",
			),
		).toBeNull();
	});

	it("allows clearing with keyboard activation", async () => {
		const { onChange } = renderAutocomplete();
		const user = userEvent.setup();

		const trigger = screen.getByRole("button", { name: /mango/i });
		const clearButton = screen.getByRole("button", {
			name: /clear selection/i,
		});

		await user.tab();
		expect(trigger).toHaveFocus();

		await user.tab();
		expect(clearButton).toHaveFocus();

		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenCalledTimes(1);
		expect(onChange).toHaveBeenCalledWith(null);
		expect(
			screen.getByRole("button", { name: /select a fruit/i }),
		).toBeInTheDocument();
	});

	it("clears the value and fires onChange when clicked", async () => {
		const { onChange } = renderAutocomplete();
		const user = userEvent.setup();
		const clearButton = screen.getByRole("button", {
			name: /clear selection/i,
		});

		await user.click(clearButton);

		expect(onChange).toHaveBeenCalledTimes(1);
		expect(onChange).toHaveBeenCalledWith(null);
		expect(
			screen.getByRole("button", { name: /select a fruit/i }),
		).toBeInTheDocument();
	});
});
