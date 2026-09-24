import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { screen, spyOn, userEvent, within } from "storybook/test";
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
// Mix single and collapsed badges across both pages.
const mockSpendUsers: OrganizationAISpendUser[] = Array.from(
	{ length: 12 },
	(_, i) => ({
		...MockOrganizationAISpendUser,
		user_id: `user-${i + 1}`,
		username: `user${String(i + 1).padStart(2, "0")}`,
		name: `User ${i + 1}`,
		cost_micros: (12 - i) * 1_000_000,
		providers: i % 3 === 0 ? ["anthropic", "openai"] : ["anthropic"],
		clients: i % 2 === 1 ? ["Claude Code", "Cursor"] : ["Claude Code"],
		models: i % 3 === 0 ? ["claude-opus-4-6", "gpt-5.4"] : ["claude-opus-4-6"],
	}),
);

const routing = { path: "/ai/settings/spend", useStoryElement: true };

// Story parameters deep-merge into the meta's, so a story cannot drop a range
// the meta sets.
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
	args: { now: fixedNow.toDate() },
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
				count: mockSpendUsers.length,
				totals: { cost_micros: 78_000_000, unpriced_usage_count: 0 },
				users: mockSpendUsers.slice(
					params.offset ?? 0,
					(params.offset ?? 0) + (params.limit ?? 10),
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

export const DefaultPeriod: Story = {
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
		await screen.findByRole("option", { name: /OpenAI/ });
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
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendUsers").mockResolvedValue({
			...MockOrganizationAISpendReport,
			count: 3,
			totals: { cost_micros: 27_000_000, unpriced_usage_count: 0 },
			users: [
				{ ...mockSpendUsers[0], providers: ["openai"] },
				{ ...mockSpendUsers[3], providers: ["openai"] },
				{ ...mockSpendUsers[6], providers: ["openai"] },
			],
		});
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

// The endpoint rejects any explicit start before the retention cutoff, so the
// picker hides the presets that reach past it and disables those days.
export const RetentionLimitedPicker: Story = {
	beforeEach: () => {
		const retentionStart = fixedNow.subtract(10, "day").toISOString();
		spyOn(API, "getOrganizationAISpendUsers").mockImplementation(
			async (_organizationId, params) => ({
				...MockOrganizationAISpendReport,
				period_start: params.period_start ?? retentionStart,
				period_end: params.period_end ?? fixedNow.toISOString(),
				retention_start: retentionStart,
				count: mockSpendUsers.length,
				users: mockSpendUsers.slice(0, 10),
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Spend by user" });
		await userEvent.click(canvas.getByRole("button", { name: "Last 7 days" }));
		await userEvent.click(
			await screen.findByRole("radio", { name: "Custom range" }),
		);
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
