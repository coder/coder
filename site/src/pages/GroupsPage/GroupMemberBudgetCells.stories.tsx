import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { getGroupByIdQueryKey } from "#/api/queries/groups";
import type { GroupMemberAISpend } from "#/api/typesGenerated";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { MockGroup2, MockGroupWithoutMembers } from "#/testHelpers/entities";
import { GroupMemberBudgetCells } from "./GroupMemberBudgetCells";

const group = MockGroupWithoutMembers;
const testId = "member-ai-budget-member-1";

const mockSpend: GroupMemberAISpend = {
	user_id: "member-1",
	effective_group_id: group.id,
	group_budget: { spend_limit_micros: 7_000_000_000, limit_source: "group" },
	group_spend_micros: 0,
};

const openInfo = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const cell = await canvas.findByTestId(testId);
	await userEvent.click(
		within(cell).getByRole("button", { name: "More info" }),
	);
	return within(document.body);
};

const meta: Meta<typeof GroupMemberBudgetCells> = {
	title: "pages/OrganizationGroupsPage/GroupMemberBudgetCells",
	component: GroupMemberBudgetCells,
	args: { group, userID: "member-1" },
	decorators: [
		(Story) => (
			<Table aria-label="Member budget">
				<TableHeader>
					<TableRow>
						<TableHead>AI budget</TableHead>
						<TableHead>Budget group</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					<TableRow>
						<Story />
					</TableRow>
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof GroupMemberBudgetCells>;

export const NoSpendData: Story = {
	args: { spend: undefined },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cells = canvas.getAllByRole("cell");
		expect(cells).toHaveLength(2);
		for (const cell of cells) {
			await expect(cell).toHaveTextContent("\u2014");
		}
	},
};

export const Unlimited: Story = {
	args: {
		spend: {
			...mockSpend,
			group_budget: null,
			effective_group_id: group.organization_id,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByTestId(testId)).toHaveTextContent(
			"Unlimited",
		);
		await expect(
			canvas.getByText("Everyone (not allocated)"),
		).toBeInTheDocument();
		const body = await openInfo(canvasElement);
		await expect(await body.findByText(/isn't restricted/)).toBeInTheDocument();
	},
};

// Unlimited where the viewed group itself is the effective group.
export const UnlimitedThisGroup: Story = {
	args: {
		spend: {
			...mockSpend,
			group_budget: null,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByTestId(testId)).toHaveTextContent(
			"Unlimited",
		);
		await expect(canvas.getByText("Front-End")).toBeInTheDocument();
	},
};

export const None: Story = {
	args: {
		spend: {
			...mockSpend,
			group_budget: { spend_limit_micros: 0, limit_source: "group" },
			effective_group_id: group.organization_id,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByTestId(testId)).toHaveTextContent("None");
		const body = await openInfo(canvasElement);
		await expect(
			await body.findByText(/no AI spending allowance/),
		).toBeInTheDocument();
	},
};

export const Regular: Story = {
	args: {
		spend: { ...mockSpend, group_spend_micros: 3_235_000_000 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("$3,235 USD");
		await expect(cell).toHaveTextContent("Group limit $7,000");
		await expect(canvas.getByText("Front-End")).toBeInTheDocument();
	},
};

export const Custom: Story = {
	args: {
		spend: {
			...mockSpend,
			group_spend_micros: 7_175_000_000,
			group_budget: {
				spend_limit_micros: 9_000_000_000,
				limit_source: "user_override",
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("$7,175 USD");
		await expect(cell).toHaveTextContent("Custom limit $9,000");
		await expect(
			canvas.getByText("Front-End (individual)"),
		).toBeInTheDocument();
	},
};

// Visual variant of Regular: the amount takes the exceeded color.
export const OverLimit: Story = {
	args: {
		spend: { ...mockSpend, group_spend_micros: 7_200_000_000 },
	},
};

export const NotAttributed: Story = {
	args: {
		spend: {
			...mockSpend,
			group_spend_micros: 456_000_000,
			effective_group_id: MockGroup2.id,
		},
	},
	parameters: {
		queries: [
			{
				key: getGroupByIdQueryKey(MockGroup2.id, { exclude_members: true }),
				data: MockGroup2,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("$456 USD");
		await expect(cell).toHaveTextContent("Not attributed to this group");
		await expect(await canvas.findByText("developer")).toBeInTheDocument();
		const body = await openInfo(canvasElement);
		await expect(
			await body.findByText(/None of this user's spend counts against/),
		).toHaveTextContent(
			"None of this user's spend counts against the Front-End group. It is managed by the developer group.",
		);
	},
};

/** Spinners while the group name resolves, not a flash of the fallback. */
export const ResolvingGroupName: Story = {
	args: {
		spend: {
			...mockSpend,
			group_spend_micros: 456_000_000,
			effective_group_id: MockGroup2.id,
		},
	},
	beforeEach: () => {
		// Never settles; the cells stay resolving.
		spyOn(API, "getGroupById").mockImplementation(() => new Promise(() => {}));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByTestId(testId);
		// Both the amount and the badge cell wait for the group name.
		await expect(canvas.getAllByTitle("Loading spinner")).toHaveLength(2);
	},
};

/** An effective group that can't be resolved, standing in for another org's. */
export const NotAttributedUnknownGroup: Story = {
	args: {
		spend: {
			...mockSpend,
			group_spend_micros: 456_000_000,
			effective_group_id: "external-group",
		},
	},
	parameters: {
		queries: [
			{
				key: getGroupByIdQueryKey("external-group", { exclude_members: true }),
				data: null,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("\u2014");
		await expect(cell).not.toHaveTextContent("$456");
		// The group cell shows an em-dash + info instead of naming the group.
		const groupCell = canvas.getAllByRole("cell")[1];
		await expect(groupCell).toHaveTextContent("\u2014");
		await userEvent.click(
			within(groupCell).getByRole("button", { name: "More info" }),
		);
		await expect(
			await within(document.body).findByText(
				/managed by a group in another organization/,
			),
		).toBeInTheDocument();
		// Close this popover so the shared message only matches once.
		await userEvent.keyboard("{Escape}");
		const body = await openInfo(canvasElement);
		await expect(
			await body.findByText(/managed by a group in another organization/),
		).toBeInTheDocument();
	},
};
