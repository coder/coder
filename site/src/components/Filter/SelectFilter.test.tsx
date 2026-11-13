import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import { SelectFilter } from "./SelectFilter";

it("selects an option with the keyboard alone", async () => {
	const user = userEvent.setup();
	const onSelect = vi.fn();
	render(
		<SelectFilter
			label="Select a status"
			placeholder="All statuses"
			options={[
				{ label: "Active", value: "active" },
				{ label: "Dormant", value: "dormant" },
			]}
			onSelect={onSelect}
		/>,
	);

	await user.tab();
	await user.keyboard("{Enter}");
	await screen.findByRole("option", { name: "Active" });
	await user.keyboard("{ArrowDown}{Enter}");
	expect(onSelect).toHaveBeenCalledWith({ label: "Dormant", value: "dormant" });
});

it("names the search input with the search label", async () => {
	const user = userEvent.setup();
	const onValueChange = vi.fn();
	render(
		<SelectFilter
			label="Select a status"
			placeholder="All statuses"
			options={[{ label: "Active", value: "active" }]}
			onSelect={vi.fn()}
			searchLabel="Search status"
			selectFilterSearch={
				<ComboboxInput
					placeholder="Search status..."
					onValueChange={onValueChange}
				/>
			}
		/>,
	);

	await user.click(screen.getByRole("button", { name: "Select a status" }));
	await user.type(
		await screen.findByRole("combobox", { name: "Search status" }),
		"a",
	);
	expect(onValueChange).toHaveBeenCalledWith("a");
});
