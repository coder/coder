import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type { AIGatewaySpendUser } from "#/api/typesGenerated";
import {
	MockAIGatewaySpendUser,
	MockAIGatewaySpendUserSummary,
	MockAIProviders,
	MockUserMember,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import SpendPage from "./SpendPage";

const fixedNow = dayjs("2026-03-12T12:00:00Z");
const users: AIGatewaySpendUser[] = Array.from({ length: 12 }, (_, i) => ({
	...MockAIGatewaySpendUser,
	id: `user-${i + 1}`,
	username: `user${String(i + 1).padStart(2, "0")}`,
	name: `User ${i + 1}`,
	total_cost_micros: (i + 1) * 1_000_000,
	request_count: 12 - i,
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
		spyOn(API, "getAIGatewaySpendUsers").mockImplementation(async (params) => {
			const field = params.sort_by ?? "total_cost_micros";
			const direction = params.sort_order === "asc" ? 1 : -1;
			const matching = users.filter(
				(user) => !params.search || user.username.includes(params.search),
			);
			matching.sort((a, b) => {
				const left = a[field];
				const right = b[field];
				return (
					direction *
					(typeof left === "string" && typeof right === "string"
						? left.localeCompare(right)
						: Number(left) - Number(right))
				);
			});
			return {
				start_date: params.start_date ?? "",
				end_date: params.end_date ?? "",
				count: matching.length,
				users: matching.slice(
					params.offset ?? 0,
					(params.offset ?? 0) + (params.limit ?? 10),
				),
			};
		});
		spyOn(API, "getAIGatewaySpendSummary").mockImplementation(
			async (params) => ({
				...MockAIGatewaySpendUserSummary,
				total_cost_micros: 78_000_000,
				start_date: params.start_date ?? "",
				end_date: params.end_date ?? "",
			}),
		);
		spyOn(API, "getAIGatewaySpendUserSummary").mockImplementation(
			async (_user, params) => ({
				...MockAIGatewaySpendUserSummary,
				start_date: params.start_date ?? "",
				end_date: params.end_date ?? "",
			}),
		);
		spyOn(API, "getUser").mockImplementation(async (id) => ({
			...MockUserMember,
			...users.find((user) => user.id === id),
		}));
		spyOn(API.experimental, "listAIProviders").mockResolvedValue(
			MockAIProviders,
		);
		spyOn(API, "getAIBridgeClients").mockResolvedValue(["Claude Code"]);
		spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o"]);
	},
} satisfies Meta<typeof SpendPage>;
export default meta;
type Story = StoryObj<typeof SpendPage>;

const FILTER_PLACEHOLDER = "Search and filter spend…";

export const SortedUsers: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const table = within(
			await canvas.findByRole("table", { name: "Spend by user" }),
		);
		await userEvent.click(table.getByRole("button", { name: "Requests" }));
		await table.findByRole("link", { name: "User 1" });
	},
};

export const SearchResults: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("combobox", { name: FILTER_PLACEHOLDER }),
			"user01",
		);
		await canvas.findByRole("link", { name: "User 1" });
	},
};

export const ProviderMenu: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: "Provider" }),
		);
		await body.findByRole("option", { name: /OpenAI/ });
	},
};

export const FilteredDrillIn: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: "Provider" }),
		);
		await userEvent.click(await body.findByRole("option", { name: /OpenAI/ }));
		await userEvent.click(await canvas.findByRole("link", { name: "User 12" }));
		await canvas.findByRole("link", { name: "View sessions" });
	},
};
