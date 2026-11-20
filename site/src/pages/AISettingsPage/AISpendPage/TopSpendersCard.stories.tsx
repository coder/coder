import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { OrganizationAISpendOverview } from "#/api/typesGenerated";
import { MockOrganizationAISpendOverview } from "#/testHelpers/entities";
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

const meta = {
	title: "pages/AISettingsPage/AISpendPage/TopSpendersCard",
	component: TopSpendersCard,
	args: { overview: mockBusyOverview, canViewSessions: true },
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
