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

// makeChatUser projects MockUserMember into a ChatUser with overridable
// per-entry share flags. Stories use it to spin up ACLs covering the
// share_tool_calls / share_attachments combinations.
const makeChatUser = (
	overrides: Partial<TypesGen.ChatUser> = {},
): TypesGen.ChatUser => ({
	id: MockUserMember.id,
	username: MockUserMember.username,
	name: MockUserMember.name ?? "",
	avatar_url: MockUserMember.avatar_url,
	role: "read",
	share_tool_calls: false,
	share_attachments: false,
	...overrides,
});

const makeChatGroup = (
	overrides: Partial<TypesGen.ChatGroup> = {},
): TypesGen.ChatGroup => ({
	...MockGroup,
	role: "read",
	share_tool_calls: false,
	share_attachments: false,
	...overrides,
});

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
// [fe-share-ok] Happy path — adding a user sends a ChatShareEntry with
// share_tool_calls and share_attachments defaulting to false.
// ---------------------------------------------------------------------------
export const AddUserDefaults: Story = {
	beforeEach: () => {
		const update = spyOn(API.experimental, "updateChatACL").mockResolvedValue();
		return () => update.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		await step("owner adds a user", async () => {
			await openPopoverAndPickUser(canvasElement);
		});
		await step("PATCH /acl fires with ChatShareEntry", async () => {
			await waitFor(() => {
				expect(API.experimental.updateChatACL).toHaveBeenCalledWith(CHAT_ID, {
					user_roles: {
						[MockUserMember.id]: {
							role: "read",
							share_tool_calls: false,
							share_attachments: false,
						},
					},
				});
			});
		});
	},
};

// ---------------------------------------------------------------------------
// All flags false — new viewers see redacted markers for both tool calls and
// attachments until the owner opts them in.
// ---------------------------------------------------------------------------
export const AllFlagsFalse: Story = {
	parameters: {
		layout: "centered",
		queries: [
			{
				key: ["chatAcl", CHAT_ID],
				data: {
					users: [makeChatUser()],
					groups: [makeChatGroup()],
				} satisfies TypesGen.ChatACL,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByTestId("chat-share-button"));
		const toolToggle = await body.findByTestId(
			`user-${MockUserMember.id}-tool-calls`,
		);
		const attachmentsToggle = await body.findByTestId(
			`user-${MockUserMember.id}-attachments`,
		);
		expect(toolToggle).toHaveAttribute("data-state", "unchecked");
		expect(attachmentsToggle).toHaveAttribute("data-state", "unchecked");
	},
};

// ---------------------------------------------------------------------------
// All flags true — owner has opted every viewer into seeing tool calls and
// attachments.
// ---------------------------------------------------------------------------
export const AllFlagsTrue: Story = {
	parameters: {
		layout: "centered",
		queries: [
			{
				key: ["chatAcl", CHAT_ID],
				data: {
					users: [
						makeChatUser({
							share_tool_calls: true,
							share_attachments: true,
						}),
					],
					groups: [
						makeChatGroup({
							share_tool_calls: true,
							share_attachments: true,
						}),
					],
				} satisfies TypesGen.ChatACL,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByTestId("chat-share-button"));
		const toolToggle = await body.findByTestId(
			`user-${MockUserMember.id}-tool-calls`,
		);
		const attachmentsToggle = await body.findByTestId(
			`user-${MockUserMember.id}-attachments`,
		);
		expect(toolToggle).toHaveAttribute("data-state", "checked");
		expect(attachmentsToggle).toHaveAttribute("data-state", "checked");
	},
};

// ---------------------------------------------------------------------------
// Mixed flags — illustrates a chat where different entries have different
// per-entry visibility settings.
// ---------------------------------------------------------------------------
export const MixedFlags: Story = {
	parameters: {
		layout: "centered",
		queries: [
			{
				key: ["chatAcl", CHAT_ID],
				data: {
					users: [
						makeChatUser({
							share_tool_calls: true,
							share_attachments: false,
						}),
					],
					groups: [
						makeChatGroup({
							share_tool_calls: false,
							share_attachments: true,
						}),
					],
				} satisfies TypesGen.ChatACL,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByTestId("chat-share-button"));
		const userTools = await body.findByTestId(
			`user-${MockUserMember.id}-tool-calls`,
		);
		const userAttachments = await body.findByTestId(
			`user-${MockUserMember.id}-attachments`,
		);
		const groupTools = await body.findByTestId(
			`group-${MockGroup.id}-tool-calls`,
		);
		const groupAttachments = await body.findByTestId(
			`group-${MockGroup.id}-attachments`,
		);
		expect(userTools).toHaveAttribute("data-state", "checked");
		expect(userAttachments).toHaveAttribute("data-state", "unchecked");
		expect(groupTools).toHaveAttribute("data-state", "unchecked");
		expect(groupAttachments).toHaveAttribute("data-state", "checked");
	},
};

// ---------------------------------------------------------------------------
// Toggling a per-entry flag sends an update carrying the full ChatShareEntry
// with the changed flag flipped and the other flag preserved.
// ---------------------------------------------------------------------------
export const ToggleToolCalls: Story = {
	parameters: {
		layout: "centered",
		queries: [
			{
				key: ["chatAcl", CHAT_ID],
				data: {
					users: [makeChatUser()],
					groups: [],
				} satisfies TypesGen.ChatACL,
			},
		],
	},
	beforeEach: () => {
		const update = spyOn(API.experimental, "updateChatACL").mockResolvedValue();
		return () => update.mockRestore();
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);
		await step("open popover and flip tool-calls toggle", async () => {
			await userEvent.click(await body.findByTestId("chat-share-button"));
			await userEvent.click(
				await body.findByTestId(`user-${MockUserMember.id}-tool-calls`),
			);
		});
		await step("PATCH carries share_tool_calls=true", async () => {
			await waitFor(() => {
				expect(API.experimental.updateChatACL).toHaveBeenLastCalledWith(
					CHAT_ID,
					{
						user_roles: {
							[MockUserMember.id]: {
								role: "read",
								share_tool_calls: true,
								share_attachments: false,
							},
						},
					},
				);
			});
		});
	},
};
