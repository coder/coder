import type { Meta, StoryObj } from "@storybook/react-vite";
import type { SelectFilterOption } from "components/Filter/SelectFilter";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "./Combobox";

const options: SelectFilterOption[] = [
	{ value: "go", label: "Go" },
	{ value: "gleam", label: "Gleam" },
	{ value: "kotlin", label: "Kotlin" },
	{ value: "rust", label: "Rust" },
];

const advancedOptions: SelectFilterOption[] = [
	{ value: "go", label: "Go", startIcon: "/icon/go.svg" },
	{ value: "gleam", label: "Gleam", startIcon: "/icon/gleam.svg" },
	{
		value: "kotlin",
		label: "Kotlin",
		startIcon: "/icon/kotlin.svg",
	},
	{ value: "rust", label: "Rust", startIcon: "/icon/rust.svg" },
];

const ComboboxWithHooks = ({
	optionsList = options,
}: {
	optionsList?: SelectFilterOption[];
}) => {
	const [value, setValue] = useState<string | undefined>(undefined);
	const selectedOption = optionsList.find((opt) => opt.value === value);

	return (
		<Combobox value={value} onValueChange={setValue}>
			<ComboboxTrigger asChild>
				<ComboboxButton
					selectedOption={selectedOption}
					placeholder="Select option"
				/>
			</ComboboxTrigger>
			<ComboboxContent className="w-60">
				<ComboboxInput placeholder="Search..." />
				<ComboboxList>
					{optionsList.map((option) => (
						<ComboboxItem key={option.value} value={option.value}>
							{option.label}
						</ComboboxItem>
					))}
				</ComboboxList>
				<ComboboxEmpty>No results found</ComboboxEmpty>
			</ComboboxContent>
		</Combobox>
	);
};

const ComboboxWithCustomValue = ({
	optionsList = options,
}: {
	optionsList?: SelectFilterOption[];
}) => {
	const [value, setValue] = useState<string | undefined>(undefined);
	const [inputValue, setInputValue] = useState("");
	const [open, setOpen] = useState(false);

	const selectedOption = optionsList.find((opt) => opt.value === value);
	const displayLabel = selectedOption?.label ?? value;

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (
			e.key === "Enter" &&
			inputValue &&
			!optionsList.some((o) => o.value === inputValue)
		) {
			setValue(inputValue);
			setInputValue("");
			setOpen(false);
		}
	};

	return (
		<Combobox
			value={value}
			onValueChange={setValue}
			open={open}
			onOpenChange={setOpen}
		>
			<ComboboxTrigger asChild>
				<ComboboxButton
					selectedOption={
						displayLabel
							? { label: displayLabel, value: value ?? "" }
							: undefined
					}
					placeholder="Select option"
				/>
			</ComboboxTrigger>
			<ComboboxContent className="w-60">
				<ComboboxInput
					placeholder="Search or enter custom..."
					value={inputValue}
					onValueChange={setInputValue}
					onKeyDown={handleKeyDown}
				/>
				<ComboboxList>
					{optionsList.map((option) => (
						<ComboboxItem key={option.value} value={option.value}>
							{option.label}
						</ComboboxItem>
					))}
				</ComboboxList>
				<ComboboxEmpty>
					<span>No results found</span>
					{inputValue && (
						<span className="block text-content-secondary text-xs mt-1">
							Press Enter to use "{inputValue}"
						</span>
					)}
				</ComboboxEmpty>
			</ComboboxContent>
		</Combobox>
	);
};

const meta: Meta<typeof Combobox> = {
	title: "components/Combobox",
	component: Combobox,
};

export default meta;
type Story = StoryObj<typeof Combobox>;

export const Default: Story = {
	render: () => <ComboboxWithHooks />,
};

export const WithAdvancedOptions: Story = {
	render: () => <ComboboxWithHooks optionsList={advancedOptions} />,
};

export const OpenCombobox: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));

		await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
	},
};

export const SelectOption: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.click(screen.getByText("Go"));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Go"),
		);
	},
};

export const SearchAndFilter: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "r");

		await waitFor(() => {
			expect(screen.getByRole("option", { name: /Rust/ })).toBeInTheDocument();
			expect(
				screen.queryByRole("option", { name: /^Go$/ }),
			).not.toBeInTheDocument();
		});
	},
};

export const WithCustomValue: Story = {
	render: () => <ComboboxWithCustomValue />,
};

export const EnterCustomValue: Story = {
	render: () => <ComboboxWithCustomValue />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "Custom Value{enter}");

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Custom Value"),
		);
	},
};

export const NoResults: Story = {
	render: () => <ComboboxWithCustomValue />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "xyz");

		await waitFor(() => {
			expect(screen.getByText("No results found")).toBeInTheDocument();
			expect(screen.getByText(/Press Enter to use/)).toBeInTheDocument();
		});
	},
};

export const ClearSelectedOption: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// First select an option
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.click(screen.getByRole("option", { name: /Go/ }));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Go"),
		);

		// Then clear it by selecting it again (toggle behavior)
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.click(screen.getByRole("option", { name: /Go/ }));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Select option"),
		);
	},
};
