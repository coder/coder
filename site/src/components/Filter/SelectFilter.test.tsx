import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { SelectFilter } from "./SelectFilter";

afterEach(() => {
	vi.restoreAllMocks();
});

it("selects an option with the keyboard alone", async () => {
	const user = userEvent.setup();
	const onSelect = vi.fn();
	renderComponent(
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
	renderComponent(
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

const longOption = { label: "Meridian Systems Engineering", value: "meridian" };

const renderLongLabelFilter = (scrollWidth: number) => {
	vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
		scrollWidth,
	);
	vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(100);
	renderComponent(
		<SelectFilter
			label="Select an organization"
			placeholder="All organizations"
			options={[longOption]}
			selectedOption={longOption}
			onSelect={vi.fn()}
		/>,
	);
	return screen.getByRole("button", { name: "Select an organization" });
};

it("describes the focused trigger with its full label when truncated", async () => {
	const user = userEvent.setup();
	const trigger = renderLongLabelFilter(200);

	await user.tab();
	await screen.findByRole("tooltip");
	expect(trigger).toHaveAccessibleDescription(longOption.label);
});

it("adds no description to the focused trigger when the label fits", async () => {
	const user = userEvent.setup();
	const trigger = renderLongLabelFilter(100);

	await user.tab();
	expect(trigger).toHaveAccessibleDescription("");
});
