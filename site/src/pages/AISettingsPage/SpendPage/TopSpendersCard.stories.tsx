import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent } from "storybook/test";
import type {
	OrganizationAISpendModel,
	OrganizationAISpendOverview,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
import {
	MockOrganizationAISpendOverview,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import { MICROS_PER_DOLLAR } from "#/utils/currency";
import { TopSpendersCard } from "./TopSpendersCard";

const [mockUser] = MockOrganizationAISpendOverview.users;

// More users have spend than the card lists, so the summary renders.
const mockBusyOverview: OrganizationAISpendOverview = {
	...MockOrganizationAISpendOverview,
	count: 44,
	totals: { spend_micros: 31_188_990_000, median_spend_micros: 231_720_000 },
	users: [
		{ name: "Maya Patel", dollars: 6_000 },
		{ name: "Jordan Lee", dollars: 5_000 },
		{ name: "Sofia Rodriguez", dollars: 4_000 },
		{ name: "Daniel Kim", dollars: 2_800 },
		{ name: "Jane Doe", dollars: 2_500 },
		{ name: "Ethan Brooks", dollars: 2_200 },
		{ name: "Leila Hassan", dollars: 1_900 },
		{ name: "Lucas Moreira", dollars: 1_600 },
		{ name: "Aisha Saeed", dollars: 1_400 },
		{ name: "Noah Chen", dollars: 1_096 },
	].map(({ name, dollars }, i) => ({
		...mockUser,
		user_id: `user-${i + 1}`,
		username: name.toLowerCase().replace(" ", "."),
		name,
		avatar_url: "",
		spend_micros: dollars * MICROS_PER_DOLLAR,
	})),
};

const mockUnpricedLlamaModel: OrganizationAISpendModel = {
	name: "llama-4-maverick",
	cost_micros: 0,
	has_unpriced_usage: true,
};

const mockUnpricedModelsByName: Record<string, OrganizationAISpendModel[]> = {
	"Maya Patel": [mockUnpricedLlamaModel],
	"Jane Doe": [mockUnpricedLlamaModel],
};

// Each user spent everything on one priced model, plus any unpriced ones.
// Jane used that model before it had a price, so it also counts as unpriced.
const mockTopSpenders: OrganizationAISpendUser[] = mockBusyOverview.users.map(
	(user) => ({
		...MockOrganizationAISpendUser,
		user_id: user.user_id,
		cost_micros: user.spend_micros,
		provider_spend: [
			{
				...MockOrganizationAISpendUser.provider_spend[0],
				cost_micros: user.spend_micros,
			},
		],
		model_spend: [
			{
				name: "claude-opus-4-6",
				cost_micros: user.spend_micros,
				has_unpriced_usage: user.name === "Jane Doe",
			},
			...(mockUnpricedModelsByName[user.name] ?? []),
		],
		client_spend: [
			{
				name: "Claude Code",
				cost_micros: user.spend_micros,
				has_unpriced_usage: false,
			},
		],
	}),
);

const meta = {
	title: "pages/AISettingsPage/SpendPage/TopSpendersCard",
	component: TopSpendersCard,
	args: {
		overview: mockBusyOverview,
		topSpenderDetails: mockTopSpenders,
		topSpenderDetailsError: null,
		organizationName: "Coder",
		canViewSessions: true,
		onOpenDetailedReport: fn(),
	},
	decorators: [
		(Story) => (
			<div className="max-w-xl">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof TopSpendersCard>;

export default meta;
type Story = StoryObj<typeof TopSpendersCard>;

export const Top10Exceeded: Story = {};

export const OneUnlistedUser: Story = {
	args: { overview: { ...mockBusyOverview, count: 11 } },
};

export const MenuOpen: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Open menu for Maya Patel" }),
		);
	},
};

export const UnpricedModelTooltipOpen: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "1 model without pricing" }),
		);
	},
};

export const UnpricedModelsTooltipOpen: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "2 models without pricing" }),
		);
	},
};

export const CostDetailsOpen: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Open menu for Jane Doe" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Cost details" }),
		);
	},
};

/** The warnings follow once the users' details load. */
export const UnpricedModelsLoading: Story = {
	args: { topSpenderDetails: undefined },
};

/** Every user with spend fits in the list, so there is no summary. */
export const AllUsersListed: Story = {
	args: { overview: MockOrganizationAISpendOverview },
};

export const SingleUser: Story = {
	args: {
		overview: {
			...MockOrganizationAISpendOverview,
			users: [mockUser],
			count: 1,
		},
	},
};

export const UserWithoutName: Story = {
	args: {
		overview: {
			...MockOrganizationAISpendOverview,
			users: [{ ...mockUser, name: "", username: "jdoe" }],
			count: 1,
		},
	},
};

export const NoSessionAccess: Story = {
	args: { canViewSessions: false },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Open menu for Jane Doe" }),
		);
	},
};

export const LongName: Story = {
	args: {
		overview: {
			...MockOrganizationAISpendOverview,
			users: [
				{
					...mockUser,
					name: "Maximiliana Alexandrina Montgomery-Fitzgerald the Third",
				},
			],
			count: 1,
		},
	},
};

export const Loading: Story = {
	args: { overview: undefined },
};

export const NoSpend: Story = {
	args: {
		overview: {
			...MockOrganizationAISpendOverview,
			count: 0,
			totals: { spend_micros: 0, median_spend_micros: 0 },
			users: [],
		},
	},
};
