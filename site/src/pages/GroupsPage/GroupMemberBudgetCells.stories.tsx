import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API, type GroupMemberAICostControl } from "#/api/api";
import { getGroupByIdQueryKey } from "#/api/queries/groups";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { MockGroup2, MockGroupWithoutMembers } from "#/testHelpers/entities";
import { emDash, GroupMemberBudgetCells } from "./GroupMemberBudgetCells";

const group = MockGroupWithoutMembers;
const testId = "member-ai-budget-member-1";

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

export const NoCostControl: Story = {
	args: { costControl: undefined },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cells = canvas.getAllByRole("cell");
		expect(cells).toHaveLength(2);
		for (const cell of cells) {
			await expect(cell).toHaveTextContent(emDash);
		}
	},
};

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

export const None: Story = {
	args: {
		costControl: costControl({
			spend_limit_micros: 0,
			effective_group_id: group.organization_id,
		}),
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

export const Custom: Story = {
	args: {
		costControl: costControl({
			current_spend_micros: 7_175_000_000,
			spend_limit_micros: 9_000_000_000,
			limit_source: "user_override",
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

// Visual variants of Regular: the amount takes the warning/exceeded color.

export const NearLimit: Story = {
	args: {
		costControl: costControl({ current_spend_micros: 6_735_000_000 }),
	},
};

export const OverLimit: Story = {
	args: {
		costControl: costControl({ current_spend_micros: 7_200_000_000 }),
	},
};

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
			await body.findByText(/None of this user's spend counts against/),
		).toHaveTextContent(
			"None of this user's spend counts against the Front-End group. It is managed by the developer group.",
		);
	},
};

// Shows a spinner while resolving, not a flash of the unresolvable fallback.
export const ResolvingGroupName: Story = {
	args: {
		costControl: costControl({
			current_spend_micros: 456_000_000,
			effective_group_id: MockGroup2.id,
		}),
	},
	beforeEach: () => {
		spyOn(API, "getGroupById").mockImplementation(
			() => new Promise(() => 1000 * 60 * 60),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cell = await canvas.findByTestId(testId);
		await expect(
			within(cell).getByTitle("Loading spinner"),
		).toBeInTheDocument();
	},
};

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
		await expect(cell).toHaveTextContent(emDash);
		await expect(cell).not.toHaveTextContent("$456");
		await expect(canvas.getByText("Another org")).toBeInTheDocument();
		const body = await openInfo(canvasElement);
		await expect(
			await body.findByText(/managed by another org and isn't visible here/),
		).toBeInTheDocument();
	},
};
