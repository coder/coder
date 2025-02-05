import type { Meta, StoryObj } from "@storybook/react";
import { expect, screen, userEvent, waitFor, within } from "@storybook/test";
import { useState } from "react";
import { Combobox } from "./Combobox";

const options = ["Option 1", "Option 2", "Option 3", "Another Option"];

const ComboboxWithHooks = () => {
	const [value, setValue] = useState("");
	const [open, setOpen] = useState(false);
	const [inputValue, setInputValue] = useState("");

	return (
		<Combobox
			value={value}
			options={options}
			placeholder="Select option"
			open={open}
			onOpenChange={setOpen}
			inputValue={inputValue}
			onInputChange={setInputValue}
			onSelect={setValue}
			onKeyDown={(e) => {
				if (e.key === "Enter" && inputValue && !options.includes(inputValue)) {
					setValue(inputValue);
					setInputValue("");
					setOpen(false);
				}
			}}
		/>
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
		await userEvent.click(
			screen.getByRole("option", { name: "Another Option" }),
		);

		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: "Another Option" }),
			).toBeInTheDocument();
			expect(
				screen.queryByRole("option", { name: "Option 1" }),
			).not.toBeInTheDocument();
		});
	},
};

export const EnterCustomValue: Story = {
	render: () => <ComboboxWithHooks />,
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
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "xyz");

		await waitFor(() => {
			expect(screen.getByText("No results found")).toBeInTheDocument();
			expect(screen.getByText("Enter custom value")).toBeInTheDocument();
		});
	},
};

export const ClearSelectedOption: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(canvas.getByRole("button"));
		// First select an option
		await userEvent.click(screen.getByRole("option", { name: "Option 1" }));
		// Then clear it by selecting it again
		await userEvent.click(screen.getByRole("option", { name: "Option 1" }));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Select option"),
		);
	},
};
