import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
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

export const SortAcrossPagesAndReturnFromUser: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const table = within(
			await canvas.findByRole("table", { name: "Spend by user" }),
		);
		await expect(table.getAllByRole("row")[1]).toHaveTextContent("User 12");
		await userEvent.click(canvas.getByRole("button", { name: "Next page" }));
		await waitFor(() =>
			expect(table.getAllByRole("row")[1]).toHaveTextContent("User 2"),
		);
		await userEvent.click(table.getByRole("button", { name: "Cost" }));
		await waitFor(() =>
			expect(API.getAIGatewaySpendUsers).toHaveBeenCalledWith(
				expect.objectContaining({
					sort_by: "total_cost_micros",
					sort_order: "asc",
					offset: 0,
				}),
			),
		);
		await waitFor(() =>
			expect(table.getAllByRole("row")[1]).toHaveTextContent("User 1"),
		);
		await expect(
			canvas.getByRole("button", { name: "Previous page" }),
		).toBeDisabled();
		await expect(canvas.getByText("$78.00")).toBeVisible();
		await userEvent.click(table.getByRole("link", { name: "User 1" }));
		await waitFor(() =>
			expect(API.getAIGatewaySpendUserSummary).toHaveBeenCalledWith("user-1", {
				start_date: "2026-02-10T00:00:00.000Z",
				end_date: "2026-03-12T00:00:00.000Z",
			}),
		);
		await expect(await canvas.findByText("$2.50")).toBeVisible();
		await expect(canvas.queryByText("$78.00")).not.toBeInTheDocument();
		await expect(
			await canvas.findByRole("table", { name: "Spend by provider" }),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Back" }));
		const returned = within(
			await canvas.findByRole("table", { name: "Spend by user" }),
		);
		await expect(
			returned.getByRole("columnheader", { name: "Cost" }),
		).toHaveAttribute("aria-sort", "ascending");
		await expect(returned.getAllByRole("row")[1]).toHaveTextContent("User 1");
		await expect(canvas.getByText("$78.00")).toBeVisible();
		await userEvent.click(returned.getByRole("button", { name: "Requests" }));
		await waitFor(() =>
			expect(API.getAIGatewaySpendUsers).toHaveBeenCalledWith(
				expect.objectContaining({
					sort_by: "request_count",
					sort_order: "desc",
				}),
			),
		);
	},
};

export const PeriodAppliesToUsersAndBreakdowns: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await canvas.findByRole("table", { name: "Spend by provider" }),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: /Feb 10, 2026.*Mar 11, 2026/ }),
		);
		await userEvent.click(
			await body.findByRole("button", { name: "Last 7 days" }),
		);
		const window = {
			start_date: fixedNow.subtract(6, "day").startOf("day").toISOString(),
			end_date: fixedNow.add(1, "hour").startOf("hour").toISOString(),
		};
		await waitFor(() =>
			expect(API.getAIGatewaySpendSummary).toHaveBeenLastCalledWith(window),
		);
		await waitFor(() =>
			expect(API.getAIGatewaySpendUsers).toHaveBeenCalledWith(
				expect.objectContaining(window),
			),
		);
		await userEvent.type(
			canvas.getByRole("textbox", { name: "Search spend by name or username" }),
			"user01",
		);
		await waitFor(() =>
			expect(API.getAIGatewaySpendUsers).toHaveBeenCalledWith(
				expect.objectContaining({ search: "user01" }),
			),
		);
		await expect(
			canvas.getByRole("table", { name: "Spend by provider" }),
		).toBeVisible();
		await expect(API.getAIGatewaySpendSummary).toHaveBeenLastCalledWith(window);
	},
};

export const FastTypedSearchIsKeptAndTrimmed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const search = await canvas.findByRole<HTMLInputElement>("textbox", {
			name: "Search spend by name or username",
		});
		await userEvent.click(search);
		// Raw input events, unlike userEvent.type, do not compensate for React
		// resetting a controlled input between keystrokes, so this mimics a fast
		// typist whose keystrokes land before the URL-driven re-render.
		const setNativeValue = Object.getOwnPropertyDescriptor(
			HTMLInputElement.prototype,
			"value",
		)?.set;
		for (const char of " user01 ") {
			setNativeValue?.call(search, search.value + char);
			search.dispatchEvent(new Event("input", { bubbles: true }));
		}
		await expect(search).toHaveValue(" user01 ");
		await waitFor(() =>
			expect(API.getAIGatewaySpendUsers).toHaveBeenCalledWith(
				expect.objectContaining({ search: "user01" }),
			),
		);
	},
};

export const ProviderFilterAppliesToUsersBreakdownsAndDrillIn: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const table = within(
			await canvas.findByRole("table", { name: "Spend by user" }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Next page" }));
		await waitFor(() =>
			expect(table.getAllByRole("row")[1]).toHaveTextContent("User 2"),
		);

		// Picking a provider filters both sections and returns to page 1.
		const providerButton = () =>
			canvas.getByRole("button", { name: "Select provider" });
		await userEvent.click(providerButton());
		await userEvent.click(await body.findByRole("option", { name: /OpenAI/ }));
		await waitFor(() =>
			expect(API.getAIGatewaySpendUsers).toHaveBeenCalledWith(
				expect.objectContaining({ provider_name: "openai", offset: 0 }),
			),
		);
		await waitFor(() =>
			expect(API.getAIGatewaySpendSummary).toHaveBeenLastCalledWith(
				expect.objectContaining({ provider_name: "openai" }),
			),
		);
		await waitFor(() =>
			expect(table.getAllByRole("row")[1]).toHaveTextContent("User 12"),
		);
		await expect(providerButton()).toHaveTextContent("OpenAI");

		await userEvent.click(await canvas.findByRole("link", { name: "User 12" }));
		await waitFor(() =>
			expect(API.getAIGatewaySpendUserSummary).toHaveBeenCalledWith(
				"user-12",
				expect.objectContaining({ provider_name: "openai" }),
			),
		);
		await expect(providerButton()).toHaveTextContent("OpenAI");

		await userEvent.click(canvas.getByRole("button", { name: "Back" }));
		await canvas.findByRole("table", { name: "Spend by user" });
		await expect(providerButton()).toHaveTextContent("OpenAI");
	},
};
