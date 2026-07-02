import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { GroupMemberAICostControl } from "#/api/api";
import { getGroupByIdQueryKey } from "#/api/queries/groups";
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

// Cost control governed by the viewed group; each story overrides per state.
const costControl = (
	overrides: Partial<GroupMemberAICostControl>,
): GroupMemberAICostControl => ({
	current_spend_micros: 0,
	spend_limit_micros: 7_000_000_000,
	effective_group_id: group.id,
	limit_source: "group",
	...overrides,
});

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

// No budget configured: governed by the everyone group, unrestricted.
export const Unlimited: Story = {
	args: {
		costControl: costControl({
			spend_limit_micros: null,
			effective_group_id: group.organization_id,
		}),
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

// Standard group budget on the viewed group.
export const Regular: Story = {
	args: {
		costControl: costControl({ current_spend_micros: 3_235_000_000 }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("$3,235 USD");
		await expect(cell).toHaveTextContent("Group limit $7,000");
		await expect(canvas.getByText("Front-End")).toBeInTheDocument();
	},
};

// Individual override on the viewed group.
export const Custom: Story = {
	args: {
		costControl: costControl({
			current_spend_micros: 7_175_000_000,
			spend_limit_micros: 9_000_000_000,
			limit_source: "override",
		}),
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

// Spend approaching the limit renders in the warning color.
export const NearLimit: Story = {
	args: {
		costControl: costControl({ current_spend_micros: 6_735_000_000 }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("$6,735 USD");
		await expect(cell).toHaveTextContent("Group limit $7,000");
	},
};

// Spend at or over the limit renders in the destructive color.
export const OverLimit: Story = {
	args: {
		costControl: costControl({ current_spend_micros: 7_200_000_000 }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(cell).toHaveTextContent("$7,200 USD");
		await expect(cell).toHaveTextContent("Group limit $7,000");
	},
};

// Governed by another group in the same org: this group's spend, unattributed.
export const NotAttributed: Story = {
	args: {
		costControl: costControl({
			current_spend_micros: 456_000_000,
			effective_group_id: MockGroup2.id,
		}),
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
			await body.findByText(/This spend happened in the/),
		).toHaveTextContent(
			"This spend happened in the Front-End group, but this user's AI budget is managed by the developer group, so it isn't counted here.",
		);
	},
};

// Governed by a group that can't be resolved (e.g. another org): no badge.
export const NotAttributedUnknownGroup: Story = {
	args: {
		costControl: costControl({
			current_spend_micros: 456_000_000,
			effective_group_id: "external-group",
		}),
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
		await expect(cell).toHaveTextContent("$456 USD");
		await expect(cell).toHaveTextContent("Not attributed to this group");
		const body = await openInfo(canvasElement);
		await expect(
			await body.findByText(/managed by another group/),
		).toBeInTheDocument();
	},
};
