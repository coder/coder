import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type { OrganizationAISpendUser } from "#/api/typesGenerated";
import {
	MockAIProviders,
	MockGroup,
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
		unpriced_usage_count: i % 4 === 0 ? 2 : 0,
	}),
);

// Mirrors the provider, model, and client filtering the endpoint applies.
const filterMockSpendUsers = (
	params: Parameters<typeof API.getOrganizationAISpendUsers>[1],
) =>
	mockSpendUsers.filter(
		(spendUser) =>
			(!params.provider_name ||
				spendUser.providers.includes(params.provider_name)) &&
			(!params.model || spendUser.models.includes(params.model)) &&
			(!params.client || spendUser.clients.includes(params.client)),
	);

const routing = { path: "/ai/settings/spend", useStoryElement: true };

const mockSpendGroup = {
	...MockGroup,
	name: "platform",
	display_name: "Platform",
	members: mockSpendUsers.slice(0, 4).map((spendUser) => ({
		...MockUserMember,
		id: spendUser.user_id,
		username: spendUser.username,
	})),
};

/** Waits until the spend table lists exactly these usernames. */
const expectSpendUsers = async (
	canvasElement: HTMLElement,
	usernames: readonly string[],
) => {
	const canvas = within(canvasElement);
	await waitFor(() => {
		const rows = within(canvas.getByRole("table", { name: "Spend by user" }))
			.getAllByRole("row")
			.slice(1);
		expect(rows.map((row) => row.textContent)).toEqual(
			usernames.map((username) => expect.stringContaining(`@${username}`)),
		);
	});
};

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
			async (_organizationId, params) => {
				const users = filterMockSpendUsers(params);
				return {
					...MockOrganizationAISpendReport,
					period_start: params.period_start ?? "2026-03-01T00:00:00.000Z",
					period_end: params.period_end ?? "2026-04-01T00:00:00.000Z",
					count: users.length,
					totals: {
						cost_micros: users.reduce((sum, u) => sum + u.cost_micros, 0),
						unpriced_usage_count: users.reduce(
							(sum, u) => sum + u.unpriced_usage_count,
							0,
						),
					},
					users: users.slice(
						params.offset ?? 0,
						(params.offset ?? 0) + (params.limit ?? 10),
					),
				};
			},
		);
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([mockSpendGroup]);
		spyOn(API, "getGroup").mockResolvedValue(mockSpendGroup);
		spyOn(API, "getAIBridgeProviders").mockResolvedValue(MockAIProviders);
		spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
		spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
		spyOn(API, "getUsers").mockResolvedValue({
			users: mockSpendUsers.map((spendUser) => ({
				...MockUserMember,
				id: spendUser.user_id,
				username: spendUser.username,
				name: spendUser.name,
			})),
			count: mockSpendUsers.length,
		});
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
			await canvas.findByRole("combobox", { name: "Search and filter users…" }),
		);
		await userEvent.hover(
			await screen.findByRole("option", { name: "Provider" }),
		);
		await screen.findByRole("button", { name: /OpenAI/ });
	},
};

const longModelNames = [
	"alibaba/qwen3-next-80b-a3b-thinking",
	"anthropic.claude-opus-4-1-20250805-v1:0",
	"arcee-ai/trinity-large-thinking-preview",
	"au.anthropic.claude-opus-4-6-20260115-v1:0",
	"gpt-4o",
];

export const ModelMenuLongNames: Story = {
	parameters: { reactRouter: explicitRange },
	beforeEach: () => {
		spyOn(API, "getAIBridgeModels").mockResolvedValue(longModelNames);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("combobox", { name: "Search and filter users…" }),
		);
		await userEvent.hover(
			await screen.findByRole("option", { name: /^Model$/ }),
		);
		await screen.findByRole("button", {
			name: "alibaba/qwen3-next-80b-a3b-thinking",
		});
	},
};

export const UnconfiguredPricingFilter: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/spend",
				searchParams: {
					startDate: "2026-02-10T00:00:00.000Z",
					endDate: "2026-03-12T00:00:00.000Z",
					filter: "pricing:unconfigured",
				},
			},
			routing,
		}),
	},
	play: async ({ canvasElement }) => {
		await expectSpendUsers(canvasElement, ["user01", "user05", "user09"]);
	},
};

export const GroupFilter: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/spend",
				searchParams: {
					startDate: "2026-02-10T00:00:00.000Z",
					endDate: "2026-03-12T00:00:00.000Z",
					filter: "group:platform",
				},
			},
			routing,
		}),
	},
	play: async ({ canvasElement }) => {
		await expectSpendUsers(canvasElement, [
			"user01",
			"user02",
			"user03",
			"user04",
		]);
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
					filter: "provider:openai",
				},
			},
			routing,
		}),
	},
	play: async ({ canvasElement }) => {
		await expectSpendUsers(canvasElement, [
			"user01",
			"user04",
			"user07",
			"user10",
		]);
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
