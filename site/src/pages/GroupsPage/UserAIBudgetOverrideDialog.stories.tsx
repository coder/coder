import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { groupAIBudget, groupsForUser } from "#/api/queries/groups";
import { getUserAIBudgetOverrideQueryKey } from "#/api/queries/users";
import type { GroupAIBudget, UserAIBudgetOverride } from "#/api/typesGenerated";
import { MockGroup, MockGroup2, MockUserMember } from "#/testHelpers/entities";
import { UserAIBudgetOverrideDialog } from "./UserAIBudgetOverrideDialog";

const mockOverride: UserAIBudgetOverride = {
	user_id: MockUserMember.id,
	group_id: MockGroup2.id,
	spend_limit_micros: 12_000_000_000,
	created_at: "2026-01-01T00:00:00Z",
	updated_at: "2026-01-01T00:00:00Z",
};

const mockGroupBudget: GroupAIBudget = {
	group_id: MockGroup.id,
	spend_limit_micros: 5_000_000_000,
	created_at: "2026-01-01T00:00:00Z",
	updated_at: "2026-01-01T00:00:00Z",
};

const groupQueries = [
	{
		key: groupsForUser(MockUserMember.id).queryKey,
		data: [MockGroup, MockGroup2],
	},
	{ key: groupAIBudget(MockGroup.id).queryKey, data: mockGroupBudget },
];

const meta: Meta<typeof UserAIBudgetOverrideDialog> = {
	title: "pages/OrganizationGroupsPage/UserAIBudgetOverrideDialog",
	component: UserAIBudgetOverrideDialog,
	args: {
		open: true,
		onOpenChange: () => undefined,
		user: MockUserMember,
		currentGroup: MockGroup,
	},
};

export default meta;
type Story = StoryObj<typeof UserAIBudgetOverrideDialog>;

export const WithOverride: Story = {
	parameters: {
		queries: [
			{
				key: getUserAIBudgetOverrideQueryKey(MockUserMember.id),
				data: mockOverride,
			},
			...groupQueries,
		],
	},
	play: async () => {
		const body = within(document.body);
		await expect(await body.findByText("AI Budget")).toBeInTheDocument();
		await expect(body.getByText("$12,000 USD")).toBeInTheDocument();
		await expect(body.getByText(/charged to/)).toBeInTheDocument();
		await expect(body.getByRole("checkbox")).toBeChecked();
		await expect(body.getByLabelText("Custom monthly budget")).toHaveValue(
			"12,000",
		);
	},
};

export const WithoutOverride: Story = {
	parameters: {
		queries: [
			{ key: getUserAIBudgetOverrideQueryKey(MockUserMember.id), data: null },
			...groupQueries,
		],
	},
	play: async () => {
		const body = within(document.body);
		await expect(await body.findByText("$5,000 USD")).toBeInTheDocument();
		await expect(body.getByText(/charged to/)).toBeInTheDocument();
		await expect(body.getByRole("checkbox")).not.toBeChecked();
		await expect(
			body.queryByLabelText("Custom monthly budget"),
		).not.toBeInTheDocument();
		await expect(
			body.queryByRole("button", { name: "Update" }),
		).not.toBeInTheDocument();
	},
};

export const ZeroBudgetDisablesAI: Story = {
	parameters: {
		queries: [
			{
				key: getUserAIBudgetOverrideQueryKey(MockUserMember.id),
				data: { ...mockOverride, spend_limit_micros: 0 },
			},
			...groupQueries,
		],
	},
	play: async () => {
		const body = within(document.body);
		await expect(body.getByLabelText("Custom monthly budget")).toHaveValue("0");
		await expect(
			await body.findByText("A $0 limit disables AI access for this member."),
		).toBeInTheDocument();
	},
};

export const SelectAssignedGroup: Story = {
	parameters: {
		queries: [
			{
				key: getUserAIBudgetOverrideQueryKey(MockUserMember.id),
				data: mockOverride,
			},
			...groupQueries,
		],
	},
	play: async ({ step }) => {
		const body = within(document.body);

		await step("open the group combobox", async () => {
			await userEvent.click(
				body.getByRole("button", { name: "Budget assigned to" }),
			);
		});

		await step("search filters the group list", async () => {
			await userEvent.type(
				await body.findByPlaceholderText("Search..."),
				"front",
			);
			await expect(
				await body.findByRole("option", { name: /Front-End \(default\)/ }),
			).toBeInTheDocument();
			await expect(
				body.queryByRole("option", { name: /developer/ }),
			).not.toBeInTheDocument();
		});

		await step("selecting a group updates the trigger", async () => {
			await userEvent.click(
				body.getByRole("option", { name: /Front-End \(default\)/ }),
			);
			await expect(
				await body.findByText("Front-End (default)"),
			).toBeInTheDocument();
		});
	},
};
