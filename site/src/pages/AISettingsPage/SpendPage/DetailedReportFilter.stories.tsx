import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import {
	MockAIProviders,
	MockGroup,
	MockGroup2,
	MockOrganization,
	MockOrganizationMember,
	MockOrganizationMember2,
} from "#/testHelpers/entities";
import { DetailedReportFilter } from "./DetailedReportFilter";

const meta = {
	title: "pages/AISettingsPage/SpendPage/DetailedReportFilter",
	component: DetailedReportFilter,
	args: {
		organizationId: MockOrganization.id,
		value: "",
		onChange: fn(),
	},
	render: function DetailedReportFilterWithState(args) {
		const [value, setValue] = useState(args.value);
		return (
			<DetailedReportFilter
				{...args}
				value={value}
				onChange={(next) => {
					setValue(next);
					args.onChange(next);
				}}
			/>
		);
	},
	beforeEach: () => {
		spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember, MockOrganizationMember2],
			count: 2,
		});
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([
			MockGroup,
			MockGroup2,
		]);
		spyOn(API, "getAIBridgeProviders").mockResolvedValue(MockAIProviders);
		spyOn(API, "getAIBridgeClients").mockResolvedValue([
			"Claude Code",
			"Codex",
		]);
		spyOn(API, "getAIBridgeModels").mockResolvedValue([
			"gpt-5.4",
			"claude-opus-4-8",
		]);
	},
} satisfies Meta<typeof DetailedReportFilter>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const WithFilters: Story = {
	args: {
		value: "user:admin group:Front-End provider:openai model:gpt-5.4",
	},
};

export const UserOptions: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Filters" }));
		await userEvent.click(await screen.findByRole("option", { name: "User" }));
	},
};

export const GroupOptions: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Filters" }));
		await userEvent.click(await screen.findByRole("option", { name: "Group" }));
	},
};

export const ProviderOptions: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Filters" }));
		await userEvent.click(
			await screen.findByRole("option", { name: "Provider" }),
		);
	},
};

export const ClientOptions: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Filters" }));
		await userEvent.click(
			await screen.findByRole("option", { name: "Client" }),
		);
	},
};

export const ModelOptions: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Filters" }));
		await userEvent.click(await screen.findByRole("option", { name: "Model" }));
	},
};

export const Searching: Story = {
	play: async () => {
		await userEvent.type(
			screen.getByRole("combobox", { name: "Search and filter…" }),
			"test",
		);
	},
};
