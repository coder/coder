import { withDesktopViewport } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "components/Avatar/Avatar";
import { ComboboxInput } from "components/Combobox/Combobox";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import { SelectFilter, type SelectFilterOption } from "./SelectFilter";

const options: SelectFilterOption[] = Array.from({ length: 50 }, (_, i) => ({
	startIcon: <Avatar fallback={`username ${i + 1}`} size="sm" />,
	label: `Option ${i + 1}`,
	value: `option-${i + 1}`,
}));

const meta: Meta<typeof SelectFilter> = {
	title: "components/SelectFilter",
	component: SelectFilter,
	args: {
		options,
		placeholder: "All options",
	},
	decorators: [withDesktopViewport],
	render: function SelectFilterWithState(args) {
		const [selectedOption, setSelectedOption] = useState<
			SelectFilterOption | undefined
		>(args.selectedOption);
		return (
			<SelectFilter
				{...args}
				selectedOption={selectedOption}
				onSelect={setSelectedOption}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export default meta;
type Story = StoryObj<typeof SelectFilter>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export const Selected: Story = {
	args: {
		selectedOption: options[25],
	},
};

export const WithSearch: Story = {
	args: {
		selectedOption: options[25],
	},
	render: function SelectFilterWithSearch(args) {
		const [selectedOption, setSelectedOption] = useState<
			SelectFilterOption | undefined
		>(args.selectedOption);
		const [search, setSearch] = useState("");

		return (
			<SelectFilter
				{...args}
				selectedOption={selectedOption}
				onSelect={setSelectedOption}
				selectFilterSearch={
					<ComboboxInput
						placeholder="Search options..."
						value={search}
						onValueChange={setSearch}
					/>
				}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export const LoadingOptions: Story = {
	args: {
		options: undefined,
	},
};

export const NoOptionsFound: Story = {
	args: {
		options: [],
	},
};

export const SelectingOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		const option = screen.getByRole("option", { name: /Option 25/ });
		await userEvent.click(option);
		await expect(button).toHaveTextContent("Option 25");
	},
};

export const UnselectingOption: Story = {
	args: {
		selectedOption: options[25],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		// Click the already-selected option to unselect it (toggle behavior)
		const option = screen.getByRole("option", { name: /Option 26/ });
		await userEvent.click(option);
		await expect(button).toHaveTextContent("All options");
	},
};

export const SearchingOption: Story = {
	render: function SelectFilterWithSearch(args) {
		const [selectedOption, setSelectedOption] = useState<
			SelectFilterOption | undefined
		>(args.selectedOption);
		const [search, setSearch] = useState("");
		const visibleOptions = options.filter((option) =>
			option.value.includes(search),
		);

		return (
			<SelectFilter
				{...args}
				selectedOption={selectedOption}
				onSelect={setSelectedOption}
				options={visibleOptions}
				selectFilterSearch={
					<ComboboxInput
						aria-label="Search options"
						placeholder="Search options..."
						value={search}
						onValueChange={setSearch}
					/>
				}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		const search = screen.getByLabelText("Search options");
		await userEvent.type(search, "option-2");
	},
};
