import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { Combobox } from "./Combobox";

const simpleOptions = ["Go", "Gleam", "Kotlin", "Rust"];

const advancedOptions = [
	{
		displayName: "Go",
		value: "go",
		icon: "/icon/go.svg",
	},
	{
		displayName: "Gleam",
		value: "gleam",
		icon: "https://github.com/gleam-lang.png",
	},
	{
		displayName: "Kotlin",
		value: "kotlin",
		description: "Kotlin 2.1, OpenJDK 24, gradle",
		icon: "/icon/kotlin.svg",
	},
	{
		displayName: "Rust",
		value: "rust",
		icon: "/icon/rust.svg",
	},
] as const;

const ComboboxWithHooks = ({
	options = advancedOptions,
}: {
	options?: React.ComponentProps<typeof Combobox>["options"];
}) => {
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
	args: { options: advancedOptions },
};

export default meta;
type Story = StoryObj<typeof Combobox>;

export const Default: Story = {};

export const SimpleOptions: Story = {
	args: {
		options: simpleOptions,
	},
};

export const OpenCombobox: Story = {
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
	},
};

export const SearchAndFilter: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "r");
		await waitFor(() => {
			expect(
				screen.queryByRole("option", { name: "Kotlin" }),
			).not.toBeInTheDocument();
		});
		await userEvent.click(screen.getByRole("option", { name: "Rust" }));
	},
};

export const EnterCustomValue: Story = {
	render: () => <ComboboxWithHooks />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await userEvent.type(screen.getByRole("combobox"), "Swift{enter}");
	},
};

export const NoResults: Story = {
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
		// const goOption = screen.getByText("Go");
		// First select an option
		await userEvent.click(await screen.findByRole("option", { name: "Go" }));
		// Then clear it by selecting it again
		await userEvent.click(await screen.findByRole("option", { name: "Go" }));

		await waitFor(() =>
			expect(canvas.getByRole("button")).toHaveTextContent("Select option"),
		);
	},
};
