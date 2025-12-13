import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Check } from "lucide-react";
import { useState } from "react";
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
							{isSelected && <Check className="size-4 shrink-0" />}
						</div>
					)}
				/>
			</div>
		);
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
							{isSelected && <Check className="size-4 shrink-0" />}
						</div>
					)}
				/>
			</div>
		);
	},
};

export const AsyncSearch: Story = {
	render: function AsyncSearchStory() {
		const [value, setValue] = useState<User | null>(null);
		const [inputValue, setInputValue] = useState("");
		const [loading, setLoading] = useState(false);
		const [filteredUsers, setFilteredUsers] = useState<User[]>([]);

		const handleInputChange = (newValue: string) => {
			setInputValue(newValue);
			setLoading(true);
			setTimeout(() => {
				const filtered = users.filter(
					(user) =>
						user.username.toLowerCase().includes(newValue.toLowerCase()) ||
						user.email.toLowerCase().includes(newValue.toLowerCase()),
				);
				setFilteredUsers(filtered);
				setLoading(false);
			}, 500);
		};

		const handleOpenChange = (open: boolean) => {
			if (open) {
				handleInputChange("");
			}
		};

		return (
			<div className="w-[350px]">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={filteredUsers}
					getOptionValue={(user) => user.id}
					getOptionLabel={(user) => user.email}
					placeholder="Search for a user"
					inputValue={inputValue}
					onInputChange={handleInputChange}
					onOpenChange={handleOpenChange}
					loading={loading}
					noOptionsText="No users found"
					renderOption={(user, isSelected) => (
						<div className="flex items-center justify-between w-full">
							<AvatarData
								title={user.username}
								subtitle={user.email}
								src={user.avatar_url}
							/>
							{isSelected && <Check className="size-4 shrink-0" />}
						</div>
					)}
				/>
			</div>
		);
	},
};

interface Country {
	code: string;
	name: string;
	flag: string;
}

const countries: Country[] = [
	{ code: "US", name: "United States", flag: "🇺🇸" },
	{ code: "GB", name: "United Kingdom", flag: "🇬🇧" },
	{ code: "CA", name: "Canada", flag: "🇨🇦" },
	{ code: "AU", name: "Australia", flag: "🇦🇺" },
	{ code: "DE", name: "Germany", flag: "🇩🇪" },
	{ code: "FR", name: "France", flag: "🇫🇷" },
	{ code: "JP", name: "Japan", flag: "🇯🇵" },
];

export const CountrySelector: Story = {
	render: function CountrySelectorStory() {
		const [value, setValue] = useState<Country | null>(null);
		return (
			<div className="w-80">
				<Autocomplete
					value={value}
					onChange={setValue}
					options={countries}
					getOptionValue={(country) => country.code}
					getOptionLabel={(country) => country.name}
					placeholder="Select a country"
					renderOption={(country, isSelected) => (
						<div className="flex items-center justify-between w-full">
							<span>
								{country.flag} {country.name}
							</span>
							{isSelected && <Check className="size-4 shrink-0" />}
						</div>
					)}
				/>
			</div>
		);
	},
};
