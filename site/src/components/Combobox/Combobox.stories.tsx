import type { Meta, StoryObj } from "@storybook/react-vite";
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

const options = [
	{ value: "option-1", label: "Option 1" },
	{ value: "option-2", label: "Option 2" },
	{ value: "option-3", label: "Option 3" },
	{ value: "another-option", label: "Another Option" },
];

const ComboboxWithHooks = () => {
	const [value, setValue] = useState<string | undefined>(undefined);
	const selectedOption = options.find((opt) => opt.value === value);

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
					{options.map((option) => (
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

const ComboboxWithCustomValue = () => {
	const [value, setValue] = useState<string | undefined>(undefined);
	const [inputValue, setInputValue] = useState("");
	const [open, setOpen] = useState(false);

	const selectedOption = options.find((opt) => opt.value === value);
	const displayLabel = selectedOption?.label ?? value;

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (
			e.key === "Enter" &&
			inputValue &&
			!options.some((o) => o.value === inputValue)
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
					{options.map((option) => (
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
		await userEvent.click(screen.getByText("Option 1"));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Option 1"),
		);
	},
};

export const SearchAndFilter: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "Another");

		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: /Another Option/ }),
			).toBeInTheDocument();
			expect(
				screen.queryByRole("option", { name: /^Option 1$/ }),
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
		await userEvent.click(screen.getByRole("option", { name: /Option 1/ }));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Option 1"),
		);

		// Then clear it by selecting it again (toggle behavior)
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.click(screen.getByRole("option", { name: /Option 1/ }));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Select option"),
		);
	},
};
