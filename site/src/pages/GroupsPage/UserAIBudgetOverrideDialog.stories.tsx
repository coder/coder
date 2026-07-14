import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
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
		key: groupsForUser(MockUserMember.id, MockGroup.organization_id).queryKey,
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
			12000,
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

export const Uncapped: Story = {
	parameters: {
		queries: [
			{ key: getUserAIBudgetOverrideQueryKey(MockUserMember.id), data: null },
			{
				key: groupsForUser(MockUserMember.id, MockGroup.organization_id)
					.queryKey,
				data: [MockGroup, MockGroup2],
			},
			{ key: groupAIBudget(MockGroup.id).queryKey, data: null },
		],
	},
	play: async ({ step }) => {
		const body = within(document.body);
		await expect(await body.findByText("uncapped")).toBeInTheDocument();
		await expect(body.getByRole("checkbox")).not.toBeChecked();

		await step(
			"enabling the override starts empty, with no $0 warning",
			async () => {
				await userEvent.click(body.getByRole("checkbox"));
				await expect(body.getByLabelText("Custom monthly budget")).toHaveValue(
					null,
				);
				await expect(
					body.queryByText("A $0 limit disables AI access for this member."),
				).not.toBeInTheDocument();
			},
		);

		await step(
			"the empty field flags an error only after it's touched",
			async () => {
				const budgetInput = body.getByLabelText("Custom monthly budget");
				await expect(
					body.queryByText("Enter a monthly budget of 0 or more."),
				).not.toBeInTheDocument();
				await userEvent.click(budgetInput);
				await userEvent.tab();
				await expect(
					await body.findByText("Enter a monthly budget of 0 or more."),
				).toBeInTheDocument();
			},
		);
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
		await expect(body.getByLabelText("Custom monthly budget")).toHaveValue(0);
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

// Clearing the budget on an enabled override blocks submit: the admin must
// enter a value or uncheck the override (to remove it) before saving.
export const SubmitRequiresValueOrUncheck: Story = {
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
		const budgetInput = await body.findByLabelText("Custom monthly budget");
		const updateButton = body.getByRole("button", { name: "Update" });

		await step("an existing override seeds a submittable value", async () => {
			await expect(updateButton).toBeEnabled();
		});

		await step("clearing the budget blocks submit", async () => {
			await userEvent.clear(budgetInput);
			// Blur to surface the error, matching the touched-then-validate flow.
			await userEvent.tab();
			await expect(
				await body.findByText("Enter a monthly budget of 0 or more."),
			).toBeInTheDocument();
			await expect(updateButton).toBeDisabled();
		});

		await step("entering a value unblocks submit", async () => {
			await userEvent.type(budgetInput, "25");
			await expect(updateButton).toBeEnabled();
		});

		await step(
			"unchecking unblocks submit to remove the override",
			async () => {
				await userEvent.clear(budgetInput);
				await expect(updateButton).toBeDisabled();
				await userEvent.click(body.getByRole("checkbox"));
				await expect(updateButton).toBeEnabled();
			},
		);
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getUserAIBudgetOverride").mockReturnValue(
			// Stay pending so the loading state renders.
			new Promise<UserAIBudgetOverride>(() => {}),
		);
	},
	parameters: { queries: groupQueries },
	play: async () => {
		const body = within(document.body);
		await expect(
			await body.findByText("Loading AI budget..."),
		).toBeInTheDocument();
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API, "getUserAIBudgetOverride").mockRejectedValue(
			new Error("test budget error"),
		);
	},
	parameters: { queries: groupQueries },
	play: async () => {
		const body = within(document.body);
		await expect(
			await body.findByText("test budget error"),
		).toBeInTheDocument();
	},
};
