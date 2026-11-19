import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { MockAIProviders } from "#/testHelpers/entities";
import { DetailedReportFilter } from "./DetailedReportFilter";

const meta = {
	title: "pages/AISettingsPage/AISpendPage/DetailedReportFilter",
	component: DetailedReportFilter,
	args: {
		value: "",
		onChange: fn(),
	},
	beforeEach: () => {
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
		value: "provider:openai model:gpt-5.4",
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

export const NoMatchingModels: Story = {
	beforeEach: () => {
		spyOn(API, "getAIBridgeModels").mockResolvedValue([]);
	},
	play: async () => {
		await userEvent.type(
			screen.getByRole("combobox", { name: "Search and filter…" }),
			"model:unknown",
		);
	},
};
