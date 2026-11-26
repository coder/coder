import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	type ComboboxOption,
	ComboboxTrigger,
} from "./Combobox";

afterEach(() => {
	vi.restoreAllMocks();
});

const statusOptions: ComboboxOption[] = [
	{ label: "Active", value: "active" },
	{ label: "Dormant", value: "dormant" },
];

type RenderComboboxOptions = {
	options?: ComboboxOption[];
	selectedOption?: ComboboxOption;
	onValueChange?: (value: string | undefined) => void;
	search?: React.ReactNode;
	searchLabel?: string;
};

const renderCombobox = ({
	options = statusOptions,
	selectedOption,
	onValueChange = vi.fn(),
	search,
	searchLabel,
}: RenderComboboxOptions = {}) =>
	renderComponent(
		<Combobox value={selectedOption?.value} onValueChange={onValueChange}>
			<ComboboxTrigger asChild>
				<ComboboxButton
					selectedOption={selectedOption}
					placeholder="All statuses"
					aria-label="Status"
				/>
			</ComboboxTrigger>
			<ComboboxContent label={searchLabel} shouldFilter={false}>
				{search}
				<ComboboxList>
					{options.map((option) => (
						<ComboboxItem key={option.value} value={option.value}>
							{option.label}
						</ComboboxItem>
					))}
				</ComboboxList>
			</ComboboxContent>
		</Combobox>,
	);

it("selects an option with the keyboard alone", async () => {
	const user = userEvent.setup();
	const onValueChange = vi.fn();
	renderCombobox({ onValueChange });

	await user.tab();
	await user.keyboard("{Enter}");
	await screen.findByRole("option", { name: "Active" });
	await user.keyboard("{ArrowDown}{Enter}");
	expect(onValueChange).toHaveBeenCalledWith("dormant");
});

it("marks the chosen option as selected", async () => {
	const user = userEvent.setup();
	renderCombobox({ selectedOption: statusOptions[1] });

	await user.click(screen.getByRole("button", { name: "Status" }));
	await screen.findByRole("option", { name: "Active" });
	expect(screen.getByRole("option", { selected: true })).toHaveAccessibleName(
		"Dormant",
	);
});

it("names the search input with the content label", async () => {
	const user = userEvent.setup();
	const onSearchChange = vi.fn();
	renderCombobox({
		searchLabel: "Search status",
		search: (
			<ComboboxInput
				placeholder="Search status..."
				onValueChange={onSearchChange}
			/>
		),
	});

	await user.click(screen.getByRole("button", { name: "Status" }));
	await user.type(
		await screen.findByRole("combobox", { name: "Search status" }),
		"a",
	);
	expect(onSearchChange).toHaveBeenCalledWith("a");
});

const longOption = { label: "Meridian Systems Engineering", value: "meridian" };

const renderLongLabelCombobox = (scrollWidth: number) => {
	vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
		scrollWidth,
	);
	vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(100);
	renderCombobox({ options: [longOption], selectedOption: longOption });
	return screen.getByRole("button", { name: "Status" });
};

it("describes the focused trigger with its full label when truncated", async () => {
	const user = userEvent.setup();
	const trigger = renderLongLabelCombobox(200);

	await user.tab();
	await screen.findByRole("tooltip");
	expect(trigger).toHaveAccessibleDescription(longOption.label);
});

it("adds no description to the focused trigger when the label fits", async () => {
	const user = userEvent.setup();
	const trigger = renderLongLabelCombobox(100);

	await user.tab();
	expect(trigger).toHaveAccessibleDescription("");
});
