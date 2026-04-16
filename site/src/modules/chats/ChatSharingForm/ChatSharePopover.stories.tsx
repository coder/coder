import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { ChatSharePopover } from "./ChatSharePopover";

const CHAT_ID = "chat-share-popover-1";

const baseChat: TypesGen.Chat = {
	id: CHAT_ID,
	organization_id: MockDefaultOrganization.id,
	owner_id: MockUserOwner.id,
	title: "Share me",
	status: "completed",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
};

const emptyACL: TypesGen.ChatACL = { users: [], groups: [] };

const meta: Meta<typeof ChatSharePopover> = {
	title: "modules/chats/ChatSharePopover",
	component: ChatSharePopover,
	args: {
		chat: baseChat,
		canShare: true,
	},
	parameters: {
		layout: "centered",
		queries: [
			{
				key: ["chatAcl", CHAT_ID],
				data: emptyACL,
			},
		],
	},
	beforeEach: () => {
		// Prime autocomplete data sources so the user picker opens fast.
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([MockGroup]);
		spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember, MockOrganizationMember2],
			count: 2,
		});
	},
};

export default meta;
type Story = StoryObj<typeof ChatSharePopover>;

const openPopoverAndPickUser = async (
	canvasElement: HTMLElement,
	username = MockUserMember.username,
) => {
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(await body.findByTestId("chat-share-button"));
	const combobox = await body.findByRole("combobox", {
		name: /search for user or group/i,
	});
	await userEvent.click(combobox);
	const option = await body.findByText(username);
	await userEvent.click(option);
	await userEvent.click(await body.findByTestId("chat-share-add-button"));
};

// ---------------------------------------------------------------------------
// [fe-share-ok] Happy path — shares with a user using "read".
// ---------------------------------------------------------------------------
export const ShareReadOnly: Story = {
	beforeEach: () => {
		const update = spyOn(API.experimental, "updateChatACL").mockResolvedValue();
		return () => update.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		await step("owner adds a user", async () => {
			await openPopoverAndPickUser(canvasElement);
		});
		await step("PATCH /acl fired with role=read", async () => {
			await waitFor(() => {
				expect(API.experimental.updateChatACL).toHaveBeenCalledWith(
					CHAT_ID,
					expect.objectContaining({
						user_roles: { [MockUserMember.id]: "read" },
						confirm_share_tool_calls: false,
						confirm_share_attachments: false,
					}),
				);
			});
		});
	},
};

// ---------------------------------------------------------------------------
// [fe-confirm-tool] 400 naming confirm_share_tool_calls opens the modal.
// ---------------------------------------------------------------------------
export const RequiresToolConfirmation: Story = {
	beforeEach: () => {
		const update = spyOn(API.experimental, "updateChatACL");
		update.mockImplementationOnce(() =>
			Promise.reject(
				mockApiError({
					message: "Chat contains tool calls that shared viewers would see.",
					detail:
						"Set confirm_share_tool_calls=true to share anyway, or clear tool-call history first.",
					validations: [
						{ field: "confirm_share_tool_calls", detail: "required" },
					],
				}),
			),
		);
		update.mockResolvedValueOnce();
		return () => update.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);
		await step("add triggers confirm modal", async () => {
			await openPopoverAndPickUser(canvasElement);
			await body.findByTestId("confirm-share-tool-calls");
		});
		await step("confirm and retry", async () => {
			await userEvent.click(
				await body.findByTestId("confirm-share-tool-calls"),
			);
			await userEvent.click(await body.findByTestId("confirm-share-submit"));
			await waitFor(() => {
				expect(API.experimental.updateChatACL).toHaveBeenLastCalledWith(
					CHAT_ID,
					expect.objectContaining({
						user_roles: { [MockUserMember.id]: "read" },
						confirm_share_tool_calls: true,
					}),
				);
			});
		});
	},
};

// ---------------------------------------------------------------------------
// [fe-confirm-attach] 400 with both confirm_share_* fields requires both
// checkboxes to be acknowledged before the retry is allowed.
// ---------------------------------------------------------------------------
export const RequiresAttachmentConfirmation: Story = {
	beforeEach: () => {
		const update = spyOn(API.experimental, "updateChatACL");
		update.mockImplementationOnce(() =>
			Promise.reject(
				mockApiError({
					message:
						"Chat contains tool calls and attachments that shared viewers would see.",
					detail:
						"Set confirm_share_tool_calls=true and confirm_share_attachments=true to share anyway, or clear the relevant history first.",
					validations: [
						{ field: "confirm_share_tool_calls", detail: "required" },
						{ field: "confirm_share_attachments", detail: "required" },
					],
				}),
			),
		);
		update.mockResolvedValueOnce();
		return () => update.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);
		await step("add triggers confirm modal", async () => {
			await openPopoverAndPickUser(canvasElement);
			await body.findByTestId("confirm-share-tool-calls");
			await body.findByTestId("confirm-share-attachments");
		});
		await step("both checkboxes required before submit", async () => {
			const submit = await body.findByTestId("confirm-share-submit");
			expect(submit).toBeDisabled();
			await userEvent.click(
				await body.findByTestId("confirm-share-tool-calls"),
			);
			expect(submit).toBeDisabled();
			await userEvent.click(
				await body.findByTestId("confirm-share-attachments"),
			);
			expect(submit).toBeEnabled();
			await userEvent.click(submit);
			await waitFor(() => {
				expect(API.experimental.updateChatACL).toHaveBeenLastCalledWith(
					CHAT_ID,
					expect.objectContaining({
						confirm_share_tool_calls: true,
						confirm_share_attachments: true,
					}),
				);
			});
		});
	},
};
