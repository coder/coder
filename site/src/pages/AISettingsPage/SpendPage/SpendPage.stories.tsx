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
const users: OrganizationAISpendUser[] = Array.from({ length: 12 }, (_, i) => ({
	...MockOrganizationAISpendUser,
	user_id: `user-${i + 1}`,
	username: `user${String(i + 1).padStart(2, "0")}`,
	name: `User ${i + 1}`,
	cost_micros: (12 - i) * 1_000_000,
	providers: i % 3 === 0 ? ["anthropic", "openai"] : ["anthropic"],
	clients: i % 2 === 0 ? ["Claude Code"] : ["Claude Code", "Cursor"],
}));

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
			location: {
				path: "/ai/settings/spend",
				searchParams: {
					startDate: "2026-02-10T00:00:00.000Z",
					endDate: "2026-03-12T00:00:00.000Z",
				},
			},
			routing: { path: "/ai/settings/spend", useStoryElement: true },
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
				total_cost_micros: 78_000_000,
				users: users
					.slice(
						params.offset ?? 0,
						(params.offset ?? 0) + (params.limit ?? 10),
					)
					// A provider filter leaves every user with that one provider.
					.map((user) =>
						params.provider_name
							? { ...user, providers: [params.provider_name] }
							: user,
					),
			}),
		);
		spyOn(API.experimental, "listAIProviders").mockResolvedValue(
			MockAIProviders,
		);
		spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
		spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
	},
} satisfies Meta<typeof SpendPage>;
export default meta;
type Story = StoryObj<typeof SpendPage>;

export const FirstPage: Story = {
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const BudgetPeriod: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/spend" },
			routing: { path: "/ai/settings/spend", useStoryElement: true },
		}),
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const ProviderMenu: Story = {
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
			routing: { path: "/ai/settings/spend", useStoryElement: true },
		}),
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("table", { name: "Spend by user" });
	},
};

export const SecondPage: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "Spend by user" });
		await userEvent.click(canvas.getByRole("button", { name: "Next page" }));
		await canvas.findByText("User 11");
	},
};
