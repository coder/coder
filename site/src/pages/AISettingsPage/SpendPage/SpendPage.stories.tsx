import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type { OrganizationAISpendUser } from "#/api/typesGenerated";
import {
	MockAIProviders,
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
	MockUserMember,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import SpendPage from "./SpendPage";

const fixedNow = dayjs("2026-03-12T12:00:00Z");
const users: OrganizationAISpendUser[] = [
	{
		...MockOrganizationAISpendUser,
		user_id: "user-1",
		username: "user01",
		name: "User 1",
		cost_micros: 12000000,
		providers: ["anthropic", "openai"],
		clients: ["Claude Code"],
		models: ["claude-opus-4-6", "gpt-5.4"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-2",
		username: "user02",
		name: "User 2",
		cost_micros: 11000000,
		providers: ["anthropic"],
		clients: ["Claude Code", "Cursor"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-3",
		username: "user03",
		name: "User 3",
		cost_micros: 10000000,
		providers: ["anthropic"],
		clients: ["Claude Code"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-4",
		username: "user04",
		name: "User 4",
		cost_micros: 9000000,
		providers: ["anthropic", "openai"],
		clients: ["Claude Code", "Cursor"],
		models: ["claude-opus-4-6", "gpt-5.4"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-5",
		username: "user05",
		name: "User 5",
		cost_micros: 8000000,
		providers: ["anthropic"],
		clients: ["Claude Code"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-6",
		username: "user06",
		name: "User 6",
		cost_micros: 7000000,
		providers: ["anthropic"],
		clients: ["Claude Code", "Cursor"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-7",
		username: "user07",
		name: "User 7",
		cost_micros: 6000000,
		providers: ["anthropic", "openai"],
		clients: ["Claude Code"],
		models: ["claude-opus-4-6", "gpt-5.4"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-8",
		username: "user08",
		name: "User 8",
		cost_micros: 5000000,
		providers: ["anthropic"],
		clients: ["Claude Code", "Cursor"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-9",
		username: "user09",
		name: "User 9",
		cost_micros: 4000000,
		providers: ["anthropic"],
		clients: ["Claude Code"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-10",
		username: "user10",
		name: "User 10",
		cost_micros: 3000000,
		providers: ["anthropic", "openai"],
		clients: ["Claude Code", "Cursor"],
		models: ["claude-opus-4-6", "gpt-5.4"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-11",
		username: "user11",
		name: "User 11",
		cost_micros: 2000000,
		providers: ["anthropic"],
		clients: ["Claude Code"],
		models: ["claude-opus-4-6"],
	},
	{
		...MockOrganizationAISpendUser,
		user_id: "user-12",
		username: "user12",
		name: "User 12",
		cost_micros: 1000000,
		providers: ["anthropic"],
		clients: ["Claude Code", "Cursor"],
		models: ["claude-opus-4-6"],
	},
];

const routing = { path: "/ai/settings/spend", useStoryElement: true };

// Story parameters deep-merge into the meta's, so the explicit range lives on
// the stories that want it and the default is the server's budget period.
const explicitRange = reactRouterParameters({
	location: {
		path: "/ai/settings/spend",
		searchParams: {
			startDate: "2026-02-10T00:00:00.000Z",
			endDate: "2026-03-12T00:00:00.000Z",
		},
	},
	routing,
});

const meta = {
	title: "pages/AISettingsPage/SpendPage/SpendPage",
	component: SpendPage,
	decorators: [withAuthProvider, withDashboardProvider],
	args: { now: fixedNow },
	parameters: {
		user: MockUserMember,
		permissions: { viewAnyAIBridgeInterception: true },
		features: ["aibridge"],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/spend" },
			routing,
		}),
	},
	beforeEach: () => {
		spyOn(API, "getOrganizations").mockResolvedValue([
			MockOrganization,
			MockOrganization2,
		]);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			[MockOrganization.id]: true,
			[MockOrganization2.id]: true,
		});
		spyOn(API, "getOrganizationAISpendUsers").mockImplementation(
			async (_organizationId, params) => ({
				...MockOrganizationAISpendReport,
				period_start: params.period_start ?? "2026-03-01T00:00:00.000Z",
				period_end: params.period_end ?? "2026-04-01T00:00:00.000Z",
				count: users.length,
				totals: { cost_micros: 78_000_000, unpriced_usage_count: 0 },
				users: users
					.slice(
						params.offset ?? 0,
						(params.offset ?? 0) + (params.limit ?? 10),
					)
					.map((user) =>
						params.provider_name
							? { ...user, providers: [params.provider_name] }
							: user,
					),
			}),
		);
		spyOn(API, "getAIBridgeProviders").mockResolvedValue(MockAIProviders);
		spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
		spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
	},
} satisfies Meta<typeof SpendPage>;
export default meta;
type Story = StoryObj<typeof SpendPage>;

export const FirstPage: Story = {
	parameters: { reactRouter: explicitRange },
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const BudgetPeriod: Story = {
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const ProviderMenu: Story = {
	parameters: { reactRouter: explicitRange },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Select provider" }),
		);
		await within(canvasElement.ownerDocument.body).findByRole("option", {
			name: /OpenAI/,
		});
	},
};

export const FilteredByProvider: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/spend",
				searchParams: {
					startDate: "2026-02-10T00:00:00.000Z",
					endDate: "2026-03-12T00:00:00.000Z",
					provider_name: "openai",
				},
			},
			routing,
		}),
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const RetentionLimitedPicker: Story = {
	parameters: { reactRouter: explicitRange },
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendUsers").mockResolvedValue({
			...MockOrganizationAISpendReport,
			retention_start: fixedNow.subtract(10, "day").toISOString(),
			count: users.length,
			users: users.slice(0, 10),
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Spend by user" });
		await userEvent.click(canvas.getByRole("button", { name: /Feb 10, 2026/ }));
		await within(canvasElement.ownerDocument.body).findByRole("button", {
			name: "Apply",
		});
	},
};

// Less than a day of retention leaves no whole day for the picker to select,
// so the report keeps the server's default window and the picker stays off.
export const SubDayRetention: Story = {
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendUsers").mockResolvedValue({
			...MockOrganizationAISpendReport,
			period_start: fixedNow.subtract(1, "hour").toISOString(),
			period_end: fixedNow.add(1, "hour").startOf("hour").toISOString(),
			retention_start: fixedNow.subtract(1, "hour").toISOString(),
			count: users.length,
			users: users.slice(0, 10),
		});
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const SecondPage: Story = {
	parameters: { reactRouter: explicitRange },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Spend by user" });
		await userEvent.click(canvas.getByRole("button", { name: "Next page" }));
	},
};
