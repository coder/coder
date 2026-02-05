import type { Meta, StoryObj } from "@storybook/react";
import { useState } from "react";
import {
	TokenSearch,
	type FilterDefinition,
	type FilterToken,
} from "./TokenSearch";

const meta: Meta<typeof TokenSearch> = {
	title: "components/TokenSearch",
	component: TokenSearch,
	parameters: {
		layout: "padded",
	},
};

export default meta;
type Story = StoryObj<typeof TokenSearch>;

// Sample filter definitions for Members page
const memberFilters: FilterDefinition[] = [
	{
		key: "role",
		label: "Role",
		options: [
			{ value: "all-perms", label: "All Perms" },
			{ value: "my-role", label: "my-role" },
			{ value: "create-others-workspaces", label: "CreateOthersWorkspaces" },
			{ value: "create-workspace", label: "Create Workspace" },
			{ value: "data-scientist", label: "Data Scientist" },
			{ value: "super-admin", label: "super-admin" },
		],
	},
	{
		key: "group",
		label: "Group",
		options: [
			{ value: "devops", label: "DevOps" },
			{ value: "tinkerers", label: "Tinkerers" },
			{ value: "prebuilt-workspaces", label: "Prebuilt Workspaces" },
			{ value: "bruno-group", label: "bruno-group" },
			{ value: "data-science", label: "Data Science" },
			{ value: "tracy-test-group", label: "tracy test group" },
			{ value: "some-group", label: "Some-group" },
		],
	},
	{
		key: "user",
		label: "User",
		options: [
			{ value: "admin", label: "Admin" },
			{ value: "tracy", label: "Tracy" },
			{ value: "bruno", label: "Bruno" },
		],
		allowCustom: true,
	},
];

// Sample filter definitions for Workspaces page
const workspaceFilters: FilterDefinition[] = [
	{
		key: "owner",
		label: "Owner",
		options: [
			{ value: "me", label: "Me" },
			{ value: "all", label: "All users" },
		],
		allowCustom: true,
	},
	{
		key: "type",
		label: "Type",
		options: [
			{ value: "workspace", label: "Workspace" },
			{ value: "dev-container", label: "Dev Container" },
		],
	},
	{
		key: "status",
		label: "Status",
		options: [
			{ value: "pending", label: "Pending" },
			{ value: "starting", label: "Starting" },
			{ value: "running", label: "Running" },
			{ value: "stopping", label: "Stopping" },
			{ value: "failed", label: "Failed" },
			{ value: "canceling", label: "Canceling" },
		],
	},
	{
		key: "name",
		label: "Name",
		options: [],
		allowCustom: true,
	},
	{
		key: "template",
		label: "Template",
		options: [
			{ value: "docker", label: "Docker" },
			{ value: "kubernetes", label: "Kubernetes" },
			{ value: "aws-ec2", label: "AWS EC2" },
			{ value: "gcp-vm", label: "GCP VM" },
		],
	},
];

// Interactive wrapper component
const TokenSearchDemo = ({
	filters,
	initialTokens = [],
	placeholder,
}: {
	filters: FilterDefinition[];
	initialTokens?: FilterToken[];
	placeholder?: string;
}) => {
	const [tokens, setTokens] = useState<FilterToken[]>(initialTokens);

	return (
		<div className="space-y-4">
			<TokenSearch
				filters={filters}
				tokens={tokens}
				onTokensChange={setTokens}
				placeholder={placeholder}
			/>
			<div className="text-sm text-content-secondary">
				<strong>Active tokens:</strong>{" "}
				{tokens.length === 0
					? "None"
					: tokens.map((t) => `${t.key}:${t.value}`).join(", ")}
			</div>
		</div>
	);
};

export const MembersSearch: Story = {
	render: () => (
		<TokenSearchDemo
			filters={memberFilters}
			placeholder="Search members..."
		/>
	),
};

export const WorkspacesSearch: Story = {
	render: () => (
		<TokenSearchDemo
			filters={workspaceFilters}
			placeholder="Search workspaces..."
		/>
	),
};

export const WithExistingTokens: Story = {
	render: () => (
		<TokenSearchDemo
			filters={memberFilters}
			initialTokens={[
				{ key: "group", value: "devops", label: "DevOps" },
				{ key: "role", value: "super-admin", label: "super-admin" },
			]}
			placeholder="Search members..."
		/>
	),
};

export const WorkspacesWithStatus: Story = {
	render: () => (
		<TokenSearchDemo
			filters={workspaceFilters}
			initialTokens={[
				{ key: "status", value: "running", label: "Running" },
			]}
			placeholder="Search workspaces..."
		/>
	),
};
