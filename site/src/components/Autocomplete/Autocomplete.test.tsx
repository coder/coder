import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Autocomplete } from "./Autocomplete";

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
