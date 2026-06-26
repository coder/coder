import type { Meta, StoryObj } from "@storybook/react-vite";
import { CheckIcon } from "lucide-react";
import { useState } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Autocomplete } from "./Autocomplete";

const meta: Meta<typeof Autocomplete> = {
	title: "components/Autocomplete",
	component: Autocomplete,
	args: {
		placeholder: "Select an option",
	},
};

export default meta;

type Story = StoryObj<typeof Autocomplete>;

interface SimpleOption {
	id: string;
	name: string;
}

const simpleOptions: SimpleOption[] = [
	{ id: "1", name: "Mango" },
	{ id: "2", name: "Banana" },
	{ id: "3", name: "Pineapple" },
	{ id: "4", name: "Kiwi" },
	{ id: "5", name: "Coconut" },
];

export const Default: Story = {
	render: function DefaultStory() {
		const [value, setValue] = useState<SimpleOption | null>(null);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={simpleOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");

		expect(trigger).toHaveTextContent("Select a fruit");
		await userEvent.click(trigger);

		await waitFor(() =>
			expect(screen.getByRole("option", { name: "Mango" })).toBeInTheDocument(),
		);
	},
};

export const WithSelectedValue: Story = {
	render: function WithSelectedValueStory() {
		const [value, setValue] = useState<SimpleOption | null>(simpleOptions[2]);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={simpleOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: /pineapple/i });
		expect(trigger).toHaveTextContent("Pineapple");

		await userEvent.click(trigger);

		await waitFor(() =>
			expect(
				screen.getByRole("option", { name: "Pineapple" }),
			).toBeInTheDocument(),
		);

		await userEvent.click(screen.getByRole("option", { name: "Mango" }));
		await waitFor(() => expect(trigger).toHaveTextContent("Mango"));
	},
};

export const NotClearable: Story = {
	render: function NotClearableStory() {
		const [value, setValue] = useState<SimpleOption | null>(simpleOptions[0]);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={simpleOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
					clearable={false}
				/>
			</div>
		);
	},
};

export const Loading: Story = {
	render: function LoadingStory() {
		const [value, setValue] = useState<SimpleOption | null>(null);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={[]}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Loading options..."
					loading
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await waitFor(() => {
			const spinners = screen.getAllByTitle("Loading spinner");
			expect(spinners.length).toBeGreaterThanOrEqual(1);
		});
	},
};

export const Disabled: Story = {
	render: function DisabledStory() {
		const [value, setValue] = useState<SimpleOption | null>(simpleOptions[1]);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={simpleOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
					disabled
				/>
			</div>
		);
	},
};

export const EmptyOptions: Story = {
	render: function EmptyOptionsStory() {
		const [value, setValue] = useState<SimpleOption | null>(null);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={[]}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
					noOptionsText="No fruits available"
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await waitFor(() =>
			expect(screen.getByText("No fruits available")).toBeInTheDocument(),
		);
	},
};

export const SearchAndFilter: Story = {
	render: function SearchAndFilterStory() {
		const [value, setValue] = useState<SimpleOption | null>(null);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={simpleOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /select a fruit/i }),
		);
		const searchInput = screen.getByRole("combobox");
		await userEvent.type(searchInput, "an");

		await waitFor(() => {
			expect(screen.getByRole("option", { name: "Mango" })).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: "Banana" }),
			).toBeInTheDocument();
			expect(
				screen.queryByRole("option", { name: "Pineapple" }),
			).not.toBeInTheDocument();
		});
	},
};

export const InlineSearch: Story = {
	args: {
		onEnterEmpty: fn<() => void>(),
	},
	render: function InlineSearchStory(args) {
		const [value, setValue] = useState<SimpleOption | null>(null);
		const [open, setOpen] = useState(false);
		const [inputValue, setInputValue] = useState("");
		const filteredOptions = simpleOptions.filter((option) =>
			option.name.toLowerCase().includes(inputValue.toLowerCase()),
		);

		const handleChange = (newValue: SimpleOption | null) => {
			setValue(newValue);
			setInputValue(newValue?.name ?? "");
		};

		return (
			<div className="w-80 space-y-2">
				<Autocomplete
					value={value}
					onChange={handleChange}
					options={filteredOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Search fruits"
					open={open}
					onOpenChange={setOpen}
					inputValue={inputValue}
					onInputChange={setInputValue}
					onEnterEmpty={() => {
						args.onEnterEmpty?.();
						setValue({ id: `custom-${inputValue}`, name: inputValue });
						setOpen(false);
					}}
					inlineSearch
					clearable={false}
					noOptionsText="No fruits found"
				/>
				<div>Selected: {value?.name ?? "None"}</div>
			</div>
		);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("combobox");
		const onEnterEmptySpy = args.onEnterEmpty as ReturnType<
			typeof fn<() => void>
		>;
		onEnterEmptySpy.mockClear();

		expect(canvas.queryByRole("button")).not.toBeInTheDocument();
		await userEvent.click(input);
		await expect(input).toHaveFocus();
		await expect(input).toHaveAttribute("aria-expanded", "true");
		await expect(
			await screen.findByRole("option", { name: "Mango" }),
		).toBeInTheDocument();

		await userEvent.type(input, "an");
		await waitFor(() => {
			expect(screen.getByRole("option", { name: "Mango" })).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: "Banana" }),
			).toBeInTheDocument();
			expect(
				screen.queryByRole("option", { name: "Pineapple" }),
			).not.toBeInTheDocument();
		});

		await userEvent.keyboard("{ArrowDown}{ArrowUp}{ArrowDown}{Enter}");
		await expect(input).toHaveFocus();
		await expect(
			await canvas.findByText("Selected: Banana"),
		).toBeInTheDocument();

		await userEvent.click(input);
		await expect(input).toHaveAttribute("aria-expanded", "true");
		await userEvent.keyboard("{Escape}");
		await waitFor(() =>
			expect(input).toHaveAttribute("aria-expanded", "false"),
		);

		await userEvent.click(input);
		await userEvent.clear(input);
		await userEvent.type(input, "dragonfruit");
		await waitFor(() => {
			expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
			expect(screen.queryByText("No fruits found")).not.toBeInTheDocument();
		});
		await expect(input).toHaveAttribute("aria-expanded", "false");

		await userEvent.keyboard("{Enter}");
		await waitFor(() => expect(onEnterEmptySpy).toHaveBeenCalledTimes(1));
		await expect(
			await canvas.findByText("Selected: dragonfruit"),
		).toBeInTheDocument();
	},
};

export const ClearSelection: Story = {
	args: {
		onChange: fn<(value: unknown) => void>(),
	},
	render: function ClearSelectionStory(args) {
		const [value, setValue] = useState<SimpleOption | null>(simpleOptions[0]);
		const handleChange = (newValue: SimpleOption | null) => {
			args.onChange(newValue);
			setValue(newValue);
		};

		return (
			<div className="w-80">
				<Autocomplete
					{...args}
					value={value}
					onChange={handleChange}
					options={simpleOptions}
					getOptionValue={(opt) => opt.id}
					getOptionLabel={(opt) => opt.name}
					placeholder="Select a fruit"
				/>
			</div>
		);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: /mango/i });
		expect(trigger).toHaveTextContent("Mango");

		const onChangeSpy = args.onChange as ReturnType<
			typeof fn<(value: unknown) => void>
		>;
		onChangeSpy.mockClear();

		const clearButton = canvas.getByLabelText("Clear selection");
		expect(clearButton).toHaveAttribute("role", "button");
		expect(clearButton).toHaveAttribute("tabindex", "0");
		expect(clearButton.tagName).toBe("SPAN");

		await userEvent.click(clearButton);
		await waitFor(() => expect(onChangeSpy).toHaveBeenCalledWith(null));

		await waitFor(() =>
			expect(
				canvas.getByRole("button", { name: /select a fruit/i }),
			).toBeInTheDocument(),
		);
	},
};

interface User {
	id: string;
	username: string;
	email: string;
	avatar_url?: string;
}

const users: User[] = [
	{
		id: "1",
		username: "alice",
		email: "alice@example.com",
		avatar_url: "",
	},
	{
		id: "2",
		username: "bob",
		email: "bob@example.com",
		avatar_url: "",
	},
	{
		id: "3",
		username: "charlie",
		email: "charlie@example.com",
		avatar_url: "",
	},
];

export const WithCustomRenderOption: Story = {
	render: function WithCustomRenderOptionStory() {
		const [value, setValue] = useState<User | null>(null);
		return (
			<div className="w-[350px]">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={users}
					getOptionValue={(user) => user.id}
					getOptionLabel={(user) => user.email}
					placeholder="Search for a user"
					renderOption={(user, isSelected) => (
						<div className="flex items-center justify-between w-full">
							<AvatarData
								title={user.username}
								subtitle={user.email}
								src={user.avatar_url}
							/>
							{isSelected && <CheckIcon className="size-4 shrink-0" />}
						</div>
					)}
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button");

		expect(trigger).toHaveTextContent("Search for a user");
		await userEvent.click(trigger);
	},
};

export const WithStartAdornment: Story = {
	render: function WithStartAdornmentStory() {
		const [value, setValue] = useState<User | null>(users[0]);
		return (
			<div className="w-[350px]">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={users}
					getOptionValue={(user) => user.id}
					getOptionLabel={(user) => user.email}
					placeholder="Search for a user"
					startAdornment={
						value && (
							<Avatar
								size="sm"
								src={value.avatar_url}
								fallback={value.username}
							/>
						)
					}
					renderOption={(user, isSelected) => (
						<div className="flex items-center justify-between w-full">
							<AvatarData
								title={user.username}
								subtitle={user.email}
								src={user.avatar_url}
							/>
							{isSelected && <CheckIcon className="size-4 shrink-0" />}
						</div>
					)}
				/>
			</div>
		);
	},
};
