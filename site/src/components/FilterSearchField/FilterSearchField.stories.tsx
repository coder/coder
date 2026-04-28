import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "components/Avatar/Avatar";
import { useState } from "react";
import {
	type FilterCategory,
	type FilterOption,
	FilterSearchField,
} from "./FilterSearchField";

const mockUsers: FilterOption[] = [
	{
		label: "me",
		value: "me",
		subtitle: "yourself",
		startIcon: <Avatar size="sm" src="" fallback="ME" />,
	},
	{
		label: "alice",
		value: "alice",
		subtitle: "alice@example.com",
		startIcon: <Avatar size="sm" src="" fallback="AL" />,
	},
	{
		label: "bob",
		value: "bob",
		subtitle: "bob@example.com",
		startIcon: <Avatar size="sm" src="" fallback="BO" />,
	},
	{
		label: "charlie",
		value: "charlie",
		subtitle: "charlie@example.com",
		startIcon: <Avatar size="sm" src="" fallback="CH" />,
	},
];

const mockStatuses: FilterOption[] = [
	{ label: "Running", value: "running" },
	{ label: "Stopped", value: "stopped" },
	{ label: "Failed", value: "failed" },
	{ label: "Starting", value: "starting" },
	{ label: "Deleting", value: "deleting" },
];

const mockTemplates: FilterOption[] = [
	{ label: "Docker", value: "docker" },
	{ label: "Kubernetes", value: "kubernetes" },
	{ label: "AWS EC2", value: "aws-ec2" },
	{ label: "Google Cloud", value: "gcp" },
];

const mockOrganizations: FilterOption[] = [
	{ label: "Default", value: "default" },
	{ label: "Engineering", value: "engineering" },
	{ label: "Design", value: "design" },
];

const defaultCategories: FilterCategory[] = [
	{
		key: "owner",
		label: "Owner",
		getOptions: async (query) => {
			await new Promise((r) => setTimeout(r, 200));
			if (!query) return mockUsers;
			return mockUsers.filter(
				(u) =>
					u.label.toLowerCase().includes(query.toLowerCase()) ||
					(u.subtitle?.toLowerCase().includes(query.toLowerCase()) ?? false),
			);
		},
	},
	{
		key: "status",
		label: "Status",
		getOptions: async (query) => {
			await new Promise((r) => setTimeout(r, 100));
			if (!query) return mockStatuses;
			return mockStatuses.filter((s) =>
				s.label.toLowerCase().includes(query.toLowerCase()),
			);
		},
	},
	{
		key: "template",
		label: "Template",
		getOptions: async (query) => {
			await new Promise((r) => setTimeout(r, 150));
			if (!query) return mockTemplates;
			return mockTemplates.filter((t) =>
				t.label.toLowerCase().includes(query.toLowerCase()),
			);
		},
	},
	{
		key: "organization",
		label: "Organization",
		getOptions: async (query) => {
			await new Promise((r) => setTimeout(r, 100));
			if (!query) return mockOrganizations;
			return mockOrganizations.filter((o) =>
				o.label.toLowerCase().includes(query.toLowerCase()),
			);
		},
	},
];

const meta: Meta<typeof FilterSearchField> = {
	title: "components/FilterSearchField",
	component: FilterSearchField,
	args: {
		placeholder: "Search workspaces...",
		categories: defaultCategories,
	},
	render: function StatefulWrapper(args) {
		const [value, setValue] = useState(args.value ?? "");
		return (
			<div className="w-full max-w-3xl">
				<FilterSearchField {...args} value={value} onChange={setValue} />
				<pre className="mt-4 text-xs text-content-secondary bg-surface-secondary p-2 rounded">
					query: "{value}"
				</pre>
			</div>
		);
	},
};

export default meta;
type Story = StoryObj<typeof FilterSearchField>;

export const Empty: Story = {};

export const WithDefaultValue: Story = {
	args: {
		value: "owner:me",
	},
};

export const MultipleFilters: Story = {
	args: {
		value: "owner:me status:running",
	},
};

export const WithFreeformText: Story = {
	args: {
		value: "owner:me my-workspace",
	},
};

export const AutoFocused: Story = {
	args: {
		autoFocus: true,
	},
};
