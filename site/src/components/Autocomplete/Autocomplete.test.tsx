import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Autocomplete } from "./Autocomplete";

const options = [
	{ id: "apple", name: "Apple" },
	{ id: "mango", name: "Mango" },
];

describe(Autocomplete.name, () => {
	it("clears the selection from the keyboard", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<Autocomplete
				value={options[1]}
				onChange={onChange}
				options={options}
				getOptionValue={(option) => option.id}
				getOptionLabel={(option) => option.name}
			/>,
		);

		await user.tab();
		expect(screen.getByRole("button", { name: "Mango" })).toHaveFocus();
		await user.tab();
		expect(
			screen.getByRole("button", { name: "Clear selection" }),
		).toHaveFocus();
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenCalledWith(null);
	});

	it("names the search input with the placeholder", async () => {
		const user = userEvent.setup();
		const onInputChange = vi.fn();
		render(
			<Autocomplete
				value={null}
				onChange={vi.fn()}
				options={["Mango"]}
				getOptionValue={(option) => option}
				getOptionLabel={(option) => option}
				placeholder="Select a fruit"
				onInputChange={onInputChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Select a fruit" }));
		await user.type(
			await screen.findByRole("combobox", { name: "Select a fruit" }),
			"m",
		);
		expect(onInputChange).toHaveBeenCalledWith("m");
	});

	it("announces while options are loading", async () => {
		const user = userEvent.setup();
		render(
			<Autocomplete
				value={null}
				onChange={vi.fn()}
				options={[]}
				getOptionValue={(option: string) => option}
				getOptionLabel={(option: string) => option}
				placeholder="Search for user or group"
				loading
			/>,
		);

		const status = screen.getByRole("status");
		expect(status).toBeEmptyDOMElement();
		await user.click(
			screen.getByRole("button", { name: /^Search for user or group/ }),
		);
		expect(status).toHaveTextContent("Loading");
	});
});

it.each([false, true])(
	"marks the chosen option as selected (inlineSearch: %s)",
	async (inlineSearch) => {
		const user = userEvent.setup();
		render(
			<Autocomplete
				value="Papaya"
				onChange={vi.fn()}
				options={["Mango", "Papaya"]}
				getOptionValue={(option) => option}
				getOptionLabel={(option) => option}
				placeholder="Select a fruit"
				inlineSearch={inlineSearch}
			/>,
		);

		if (inlineSearch) {
			await user.click(screen.getByRole("combobox"));
		} else {
			await user.click(screen.getByRole("button", { name: /Papaya/ }));
		}
		await screen.findByRole("option", { name: "Mango" });
		expect(screen.getByRole("option", { selected: true })).toHaveAccessibleName(
			"Papaya",
		);
	},
);
