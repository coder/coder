import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "react-query";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { UserOverridesSection } from "./UserOverridesSection";

const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			retry: false,
			gcTime: 0,
			refetchOnWindowFocus: false,
		},
	},
});

const mockOverrides = [
	{
		user_id: "user-1",
		name: "Alice Johnson",
		username: "alice",
		avatar_url: "",
		spend_limit_micros: 5_000_000,
	},
	{
		user_id: "user-2",
		name: "Bob Smith",
		username: "bob",
		avatar_url: "",
		spend_limit_micros: 10_000_000,
	},
	{
		user_id: "user-3",
		name: "Charlie Davis",
		username: "charlie",
		avatar_url: "",
		spend_limit_micros: null,
	},
];

const meta: Meta<typeof UserOverridesSection> = {
	title: "pages/AgentsPage/LimitsTab/UserOverridesSection",
	component: UserOverridesSection,
	args: {
		overrides: mockOverrides,
		showUserForm: false,
		onShowUserFormChange: fn(),
		selectedUser: null,
		onSelectedUserChange: fn(),
		userOverrideAmount: "",
		onUserOverrideAmountChange: fn(),
		selectedUserAlreadyOverridden: false,
		editingUserOverride: null,
		onEditUserOverride: fn(),
		onAddOverride: fn(),
		onDeleteOverride: fn(),
		upsertPending: false,
		upsertError: null,
		deletePending: false,
		deleteError: null,
	},
	decorators: [
		(Story) => (
			<QueryClientProvider client={queryClient}>
				<Story />
			</QueryClientProvider>
		),
	],
};

export default meta;
type Story = StoryObj<typeof UserOverridesSection>;

export const Default: Story = {};

export const EmptyState: Story = {
	args: {
		overrides: [],
	},
};

export const AddForm: Story = {
	args: {
		showUserForm: true,
		overrides: mockOverrides,
	},
};

export const EditForm: Story = {
	args: {
		showUserForm: true,
		editingUserOverride: {
			user_id: mockOverrides[0].user_id,
			name: mockOverrides[0].name,
			username: mockOverrides[0].username,
			avatar_url: mockOverrides[0].avatar_url,
		},
		userOverrideAmount: "5.00",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Save button confirms edit mode is active.
		await expect(
			canvas.getByRole("button", { name: /save/i }),
		).toBeInTheDocument();
		// The editing user name appears in both the table row and the
		// read-only edit form identity, confirming it was populated.
		const nameElements = canvas.getAllByText("Alice Johnson");
		expect(nameElements.length).toBeGreaterThanOrEqual(2);
	},
};

export const DeleteUserOverride: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the Delete button for the first user override.
		const deleteButtons = await body.findAllByRole("button", {
			name: "Delete",
		});
		await userEvent.click(deleteButtons[0]);

		// The confirmation dialog should appear.
		const dialog = await body.findByRole("dialog");
		await expect(dialog).toBeInTheDocument();
		await expect(
			body.getByText(
				/Are you sure you want to delete this user limit override/i,
			),
		).toBeInTheDocument();
	},
};

export const DeleteUserOverrideCancelled: Story = {
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click the Delete button for the first user override.
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
		expect(args.onDeleteOverride).not.toHaveBeenCalled();
	},
};
