import type { Meta, StoryObj } from "@storybook/react";
import { useState } from "react";
import {
	SearchableSelect,
	SearchableSelectContent,
	SearchableSelectItem,
	SearchableSelectTrigger,
	SearchableSelectValue,
} from "./SearchableSelect";
import { GitBranch, Globe, Lock, Users } from "lucide-react";

const meta: Meta<typeof SearchableSelect> = {
	title: "components/SearchableSelect",
	component: SearchableSelect,
	args: {
		placeholder: "Select an option",
	},
};

export default meta;
type Story = StoryObj<typeof SearchableSelect>;

const SimpleOptions = () => {
	const [value, setValue] = useState("");

	return (
		<SearchableSelect value={value} onValueChange={setValue}>
			<SearchableSelectTrigger>
				<SearchableSelectValue />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
				<SearchableSelectItem value="option2">Option 2</SearchableSelectItem>
				<SearchableSelectItem value="option3">Option 3</SearchableSelectItem>
				<SearchableSelectItem value="option4">Option 4</SearchableSelectItem>
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const Default: Story = {
	render: () => <SimpleOptions />,
};

const ManyOptionsExample = () => {
	const [value, setValue] = useState("");
	const options = Array.from({ length: 50 }, (_, i) => ({
		value: `option-${i + 1}`,
		label: `Option ${i + 1}`,
	}));

	return (
		<SearchableSelect
			value={value}
			onValueChange={setValue}
			placeholder="Search from many options..."
		>
			<SearchableSelectTrigger>
				<SearchableSelectValue />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				{options.map((option) => (
					<SearchableSelectItem key={option.value} value={option.value}>
						{option.label}
					</SearchableSelectItem>
				))}
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const WithManyOptions: Story = {
	render: () => <ManyOptionsExample />,
};

const WithIconsExample = () => {
	const [value, setValue] = useState("");

	return (
		<SearchableSelect value={value} onValueChange={setValue}>
			<SearchableSelectTrigger>
				<SearchableSelectValue placeholder="Select visibility" />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				<SearchableSelectItem value="public">
					<div className="flex items-center gap-2">
						<Globe className="size-icon-sm" />
						<span>Public</span>
					</div>
				</SearchableSelectItem>
				<SearchableSelectItem value="private">
					<div className="flex items-center gap-2">
						<Lock className="size-icon-sm" />
						<span>Private</span>
					</div>
				</SearchableSelectItem>
				<SearchableSelectItem value="team">
					<div className="flex items-center gap-2">
						<Users className="size-icon-sm" />
						<span>Team only</span>
					</div>
				</SearchableSelectItem>
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const WithIcons: Story = {
	render: () => <WithIconsExample />,
};

const ProgrammingLanguagesExample = () => {
	const [value, setValue] = useState("");
	const languages = [
		"JavaScript", "TypeScript", "Python", "Java", "C++", "C#", "Ruby",
		"Go", "Rust", "Swift", "Kotlin", "Scala", "PHP", "Perl", "R",
		"MATLAB", "Julia", "Dart", "Lua", "Haskell", "Clojure", "Elixir",
		"F#", "OCaml", "Erlang", "Nim", "Crystal", "Zig", "V", "Racket"
	];

	return (
		<SearchableSelect
			value={value}
			onValueChange={setValue}
			placeholder="Select a programming language"
		>
			<SearchableSelectTrigger>
				<SearchableSelectValue />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				{languages.map((lang) => (
					<SearchableSelectItem key={lang} value={lang.toLowerCase()}>
						{lang}
					</SearchableSelectItem>
				))}
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const ProgrammingLanguages: Story = {
	render: () => <ProgrammingLanguagesExample />,
};

const DisabledExample = () => {
	return (
		<SearchableSelect value="disabled" disabled>
			<SearchableSelectTrigger>
				<SearchableSelectValue />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				<SearchableSelectItem value="disabled">Disabled Option</SearchableSelectItem>
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const Disabled: Story = {
	render: () => <DisabledExample />,
};

const RequiredExample = () => {
	const [value, setValue] = useState("");

	return (
		<form onSubmit={(e) => { e.preventDefault(); alert(`Selected: ${value}`); }}>
			<div className="space-y-4">
				<SearchableSelect
					value={value}
					onValueChange={setValue}
					required
					placeholder="This field is required"
				>
					<SearchableSelectTrigger>
						<SearchableSelectValue />
					</SearchableSelectTrigger>
					<SearchableSelectContent>
						<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
						<SearchableSelectItem value="option2">Option 2</SearchableSelectItem>
						<SearchableSelectItem value="option3">Option 3</SearchableSelectItem>
					</SearchableSelectContent>
				</SearchableSelect>
				<button type="submit" className="px-4 py-2 bg-content-link text-white rounded">
					Submit
				</button>
			</div>
		</form>
	);
};

export const Required: Story = {
	render: () => <RequiredExample />,
};

const EmptyStateExample = () => {
	const [value, setValue] = useState("");

	return (
		<SearchableSelect
			value={value}
			onValueChange={setValue}
			emptyMessage="No matching options found. Try a different search term."
		>
			<SearchableSelectTrigger>
				<SearchableSelectValue placeholder="Type to search..." />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				{/* Intentionally empty to show empty state */}
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const EmptyState: Story = {
	render: () => <EmptyStateExample />,
};

const GitBranchesExample = () => {
	const [value, setValue] = useState("main");
	const branches = [
		{ name: "main", isDefault: true },
		{ name: "develop", isDefault: false },
		{ name: "feature/user-authentication", isDefault: false },
		{ name: "feature/payment-integration", isDefault: false },
		{ name: "bugfix/header-alignment", isDefault: false },
		{ name: "hotfix/security-patch", isDefault: false },
		{ name: "release/v2.0.0", isDefault: false },
		{ name: "chore/update-dependencies", isDefault: false },
	];

	return (
		<SearchableSelect
			value={value}
			onValueChange={setValue}
			placeholder="Select a branch"
		>
			<SearchableSelectTrigger className="w-72">
				<SearchableSelectValue />
			</SearchableSelectTrigger>
			<SearchableSelectContent>
				{branches.map((branch) => (
					<SearchableSelectItem key={branch.name} value={branch.name}>
						<div className="flex items-center gap-2">
							<GitBranch className="size-icon-sm" />
							<span>{branch.name}</span>
							{branch.isDefault && (
								<span className="ml-auto text-xs text-content-secondary">default</span>
							)}
						</div>
					</SearchableSelectItem>
				))}
			</SearchableSelectContent>
		</SearchableSelect>
	);
};

export const GitBranches: Story = {
	render: () => <GitBranchesExample />,
};
