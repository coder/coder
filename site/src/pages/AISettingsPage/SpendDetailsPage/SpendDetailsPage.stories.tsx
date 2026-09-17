import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	MockAIProviders,
	MockGroup,
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendDetails,
	MockOrganizationAISpendRow,
	MockOrganizationMember,
	MockUserMember,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import SpendDetailsPage from "./SpendDetailsPage";

const response = MockOrganizationAISpendDetails;

const meta = {
	title: "pages/AISettingsPage/SpendDetailsPage",
	component: SpendDetailsPage,
	decorators: [withAuthProvider, withDashboardProvider],
	args: { now: new Date("2026-09-16T12:00:00Z") },
	parameters: {
		user: MockUserMember,
		permissions: { viewAnyAIBridgeInterception: true },
		features: ["aibridge"],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/spend-details" },
			routing: { path: "/ai/settings/spend-details", useStoryElement: true },
		}),
	},
	beforeEach: () => {
		spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			[MockOrganization.id]: true,
		});
		spyOn(API, "getOrganizationAISpendDetails").mockResolvedValue(response);
		spyOn(API, "exportOrganizationAISpend").mockResolvedValue(
			new Blob(["csv"]),
		);
		spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember],
			count: 1,
		});
		spyOn(API, "getOrganizationPaginatedGroups").mockResolvedValue({
			groups: [MockGroup],
			count: 1,
		});
		spyOn(API.experimental, "listAIProviders").mockResolvedValue(
			MockAIProviders,
		);
		spyOn(API, "getAIBridgeModels").mockResolvedValue(["gpt-4o-mini"]);
	},
} satisfies Meta<typeof SpendDetailsPage>;

export default meta;
type Story = StoryObj<typeof SpendDetailsPage>;

export const Default: Story = {};

export const MultipleOrganizations: Story = {
	beforeEach: () => {
		const organizations = [MockOrganization, MockOrganization2];
		spyOn(API, "getOrganizations").mockResolvedValue(organizations);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			[MockOrganization.id]: true,
			[MockOrganization2.id]: true,
		});
		spyOn(API, "getOrganizationAISpendDetails").mockImplementation(
			async (organizationId) => {
				const organization =
					organizations.find((org) => org.id === organizationId) ??
					MockOrganization;
				return {
					...response,
					rows: [
						{
							...MockOrganizationAISpendRow,
							organization_id: organization.id,
							organization_name: organization.name,
							username:
								organization.id === MockOrganization.id ? "alice" : "bob",
							cost_micros:
								organization.id === MockOrganization.id ? 1_250_000 : 7_500_000,
						},
					],
				};
			},
		);
	},
};

const paginatedRows = Array.from({ length: 58 }, (_, index) => ({
	...MockOrganizationAISpendRow,
	user_id: `spend-user-${index + 1}`,
	username: `user-${String(index + 1).padStart(2, "0")}`,
	input_tokens: (index + 1) * 1_000,
	output_tokens: (index + 1) * 200,
	cache_read_tokens: (index + 1) * 400,
	cache_write_tokens: (index + 1) * 50,
	cost_micros: (index + 1) * 250_000,
}));

export const Paginated: Story = {
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendDetails").mockImplementation(
			async (_organizationId, { limit = 25, offset = 0 }) => ({
				...response,
				count: paginatedRows.length,
				rows: paginatedRows.slice(offset, offset + limit),
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("user-01");
		await userEvent.click(canvas.getByRole("button", { name: /next page/i }));
		await canvas.findByText("user-26");
	},
};

export const ProviderAndModelIcons: Story = {
	beforeEach: () => {
		const rows = [
			MockOrganizationAISpendRow,
			{
				...MockOrganizationAISpendRow,
				provider: "anthropic",
				provider_name: "Anthropic production",
				model: "claude-sonnet-4-5",
			},
			{
				...MockOrganizationAISpendRow,
				provider: "custom",
				provider_name: "Custom provider",
				model: "custom-model",
			},
		];
		spyOn(API, "getOrganizationAISpendDetails").mockResolvedValue({
			...response,
			count: rows.length,
			rows,
		});
	},
};

export const ProviderMenu: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", {
				name: "Select provider",
			}),
		);
	},
};

export const ExportError: Story = {
	beforeEach: () => {
		spyOn(API, "exportOrganizationAISpend").mockRejectedValue(
			new Error("Unable to export spend details"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("table", { name: "AI spend details" });
		await userEvent.click(canvas.getByRole("button", { name: "Export CSV" }));
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendDetails").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendDetails").mockRejectedValue(
			new Error("Unable to load spend details"),
		);
	},
};

export const Unlicensed: Story = {
	parameters: { features: [] },
};

export const GatewayDisabled: Story = {
	parameters: { features: [{ name: "aibridge", enabled: false }] },
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API, "getOrganizationAISpendDetails").mockResolvedValue({
			...response,
			count: 0,
			rows: [],
		});
	},
};
