import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { Group } from "#/api/typesGenerated";
import { GroupLimitsSection } from "./GroupLimitsSection";

const mockGroupOverrides = [
	{
		group_id: "group-1",
		group_display_name: "Engineering",
		group_name: "engineering",
		group_avatar_url: "",
		member_count: 15,
		spend_limit_micros: 10_000_000,
	},
	{
		group_id: "group-2",
		group_display_name: "Design",
		group_name: "design",
		group_avatar_url: "",
		member_count: 8,
		spend_limit_micros: 5_000_000,
	},
	{
		group_id: "group-4",
		group_display_name: "Support",
		group_name: "support",
		group_avatar_url: "",
		member_count: 11,
		spend_limit_micros: null,
	},
];

const mockAvailableGroups: Group[] = [
	{
		id: "group-3",
		name: "marketing",
		display_name: "Marketing",
		avatar_url: "",
		organization_id: "org-1",
		organization_name: "Acme",
		organization_display_name: "Acme",
		members: [],
		quota_allowance: 0,
		source: "user",
		total_member_count: 5,
	},
	{
		id: "group-5",
		name: "sales",
		display_name: "Sales",
		avatar_url: "",
		organization_id: "org-1",
		organization_name: "Acme",
		organization_display_name: "Acme",
		members: [],
		quota_allowance: 0,
		source: "user",
		total_member_count: 12,
	},
];

const editingGroupOverride = {
	group_id: mockGroupOverrides[0].group_id,
	group_display_name: mockGroupOverrides[0].group_display_name,
	group_name: mockGroupOverrides[0].group_name,
	group_avatar_url: mockGroupOverrides[0].group_avatar_url,
	member_count: mockGroupOverrides[0].member_count,
};

const meta: Meta<typeof GroupLimitsSection> = {
	title: "pages/AgentsPage/LimitsTab/GroupLimitsSection",
	component: GroupLimitsSection,
	args: {
		groupOverrides: mockGroupOverrides,
		groupOrganizationNames: {
			"group-1": "acme",
			"group-2": "acme",
			"group-4": "acme",
		},
		showGroupForm: false,
		onShowGroupFormChange: fn(),
		selectedGroup: null,
		onSelectedGroupChange: fn(),
		groupAmount: "",
		onGroupAmountChange: fn(),
		availableGroups: [],
		groupAutocompleteNoOptionsText: "No groups available",
		groupsLoading: false,
		editingGroupOverride: null,
		onEditGroupOverride: fn(),
		onAddGroupOverride: fn(),
		onDeleteGroupOverride: fn(),
		upsertPending: false,
		upsertError: null,
		deletePending: false,
		deleteError: null,
		groupsError: null,
	},
};

export default meta;
type Story = StoryObj<typeof GroupLimitsSection>;

export const Default: Story = {};

export const EmptyState: Story = {
	args: {
		groupOverrides: [],
	},
};

export const AddForm: Story = {
	args: {
		showGroupForm: true,
		availableGroups: mockAvailableGroups,
	},
};

export const EditForm: Story = {
	args: {
		showGroupForm: true,
		editingGroupOverride,
		groupAmount: "10.00",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Save button confirms edit mode is active.
		await expect(
			canvas.getByRole("button", { name: /save/i }),
		).toBeInTheDocument();
		// The editing group name appears in both the table row and the
		// read-only edit form identity, confirming it was populated.
		const nameElements = canvas.getAllByText(
			editingGroupOverride.group_display_name,
		);
		expect(nameElements.length).toBeGreaterThanOrEqual(2);
	},
};

export const DeleteGroupOverride: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the Delete button for the first group override.
		const deleteButtons = await body.findAllByRole("button", {
			name: "Delete",
		});
		await userEvent.click(deleteButtons[0]);

		// The confirmation dialog should appear.
		const dialog = await body.findByRole("dialog");
		await expect(dialog).toBeInTheDocument();
		await expect(
			body.getByText(/Are you sure you want to delete this group override/i),
		).toBeInTheDocument();
	},
};

export const DeleteGroupOverrideCancelled: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the Delete button for the first group override.
		const deleteButtons = await body.findAllByRole("button", {
			name: "Delete",
		});
		await userEvent.click(deleteButtons[0]);

		// Cancel the dialog.
		await body.findByRole("dialog");
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		// The dialog should be closed and the callback should not have been called.
		await waitFor(() => {
			expect(body.queryByRole("dialog")).not.toBeInTheDocument();
		});
		expect(args.onDeleteGroupOverride).not.toHaveBeenCalled();
	},
};
