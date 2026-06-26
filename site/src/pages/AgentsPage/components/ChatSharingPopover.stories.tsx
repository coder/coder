import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockGroup,
	MockGroup2,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import { ChatShareButton } from "./ChatSharingPopover";

const chatId = "chat-1";
const organizationId = MockDefaultOrganization.id;

const currentChatUser: TypesGen.ChatUser = {
	id: MockUserOwner.id,
	username: MockUserOwner.username,
	name: MockUserOwner.name,
	avatar_url: MockUserOwner.avatar_url,
	role: "read",
};

const chatUser: TypesGen.ChatUser = {
	id: MockUserMember.id,
	username: MockUserMember.username,
	name: MockUserMember.name,
	avatar_url: MockUserMember.avatar_url,
	role: "read",
};

const chatGroup: TypesGen.ChatGroup = {
	...MockGroup,
	role: "read",
};

const emptyACL: TypesGen.ChatACL = { users: [], groups: [] };
const populatedACL: TypesGen.ChatACL = {
	users: [chatUser],
	groups: [chatGroup],
};

const ignoreResizeObserverLoopError = () => {
	const handleError = (event: ErrorEvent) => {
		if (
			event.message ===
			"ResizeObserver loop completed with undelivered notifications."
		) {
			event.stopImmediatePropagation();
			event.preventDefault();
		}
	};

	window.addEventListener("error", handleError);
	return () => window.removeEventListener("error", handleError);
};

type LookupRequestMocks = {
	acl?: TypesGen.ChatACL;
	aclError?: Error;
	aclPending?: boolean;
};

const mockLookupRequests = ({
	acl = emptyACL,
	aclError,
	aclPending,
}: LookupRequestMocks = {}) => {
	if (aclPending) {
		spyOn(API.experimental, "getChatACL").mockReturnValue(
			new Promise<TypesGen.ChatACL>(() => undefined),
		);
	} else if (aclError) {
		spyOn(API.experimental, "getChatACL").mockRejectedValue(aclError);
	} else {
		spyOn(API.experimental, "getChatACL").mockResolvedValue(acl);
	}

	spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
		members: [MockOrganizationMember, MockOrganizationMember2],
		count: 2,
	});
	spyOn(API, "getGroupsByOrganization").mockResolvedValue([
		MockGroup,
		MockGroup2,
	]);

	return ignoreResizeObserverLoopError();
};

type DialogRequestMocks = LookupRequestMocks & {
	updateError?: Error;
};

const mockDialogRequests = ({
	updateError,
	...lookupOptions
}: DialogRequestMocks = {}) => {
	const cleanup = mockLookupRequests(lookupOptions);

	if (updateError) {
		spyOn(API.experimental, "updateChatACL").mockRejectedValue(updateError);
	} else {
		spyOn(API.experimental, "updateChatACL").mockResolvedValue();
	}

	return cleanup;
};

const openChatSharing = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button", { name: "Share" }));
	const body = within(canvasElement.ownerDocument.body);
	await body.findByText("Chat sharing");
	return body;
};

const closeChatSharing = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(canvas.getByRole("button", { name: "Share" }));
	await waitFor(() => {
		expect(body.queryByText("Chat sharing")).not.toBeInTheDocument();
	});
};

const addAutocompleteOption = async (
	body: ReturnType<typeof within>,
	{
		query,
		option,
		triggerName = "Search for user or group",
	}: {
		query: string;
		option: string | RegExp;
		triggerName?: string | RegExp;
	},
) => {
	await userEvent.click(await body.findByRole("button", { name: triggerName }));
	await userEvent.type(
		body.getByPlaceholderText("Search for user or group"),
		query,
	);
	await userEvent.click(await body.findByRole("option", { name: option }));
	await userEvent.click(body.getByRole("button", { name: "Add member" }));
};

const MobileFrame = (Story: React.FC) => (
	<div className="w-[390px] max-w-full">
		<Story />
	</div>
);

const meta: Meta<typeof ChatShareButton> = {
	title: "pages/AgentsPage/ChatSharingPopover",
	component: ChatShareButton,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserOwner,
	},
	args: {
		chatId,
		organizationId,
	},
};

export default meta;
type Story = StoryObj<typeof ChatShareButton>;

export const EmptyACL: Story = {
	beforeEach: () => mockDialogRequests(),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(
				body.getAllByText("No shared members or groups yet").length,
			).toBeGreaterThan(0);
			expect(
				body.getAllByText("Add a member or group using the controls above.")
					.length,
			).toBeGreaterThan(0);
		});
	},
};

export const PopulatedACL: Story = {
	beforeEach: () => mockDialogRequests({ acl: populatedACL }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(body.getAllByText(chatUser.username).length).toBeGreaterThan(0);
			expect(body.getAllByText(chatGroup.name).length).toBeGreaterThan(0);
			expect(body.getAllByText("Read").length).toBeGreaterThan(0);
		});
		expect(
			body.getAllByRole("button", { name: "Open menu" }).length,
		).toBeGreaterThan(0);
	},
};

export const MobilePopulatedACL: Story = {
	decorators: [MobileFrame],
	parameters: {
		chromatic: { viewports: [390] },
	},
	beforeEach: () => mockDialogRequests({ acl: populatedACL }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(body.getAllByText(chatUser.username).length).toBeGreaterThan(0);
			expect(body.getAllByText(chatGroup.name).length).toBeGreaterThan(0);
			expect(body.getAllByText("Read").length).toBeGreaterThan(0);
		});
		expect(
			body.getAllByRole("button", { name: "Open menu" }).length,
		).toBeGreaterThan(0);
	},
};

export const CurrentUserHidden: Story = {
	beforeEach: () =>
		mockDialogRequests({
			acl: {
				users: [currentChatUser, chatUser],
				groups: [],
			},
		}),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(
				body.queryByText(currentChatUser.username),
			).not.toBeInTheDocument();
			expect(body.getAllByText(chatUser.username).length).toBeGreaterThan(0);
		});
	},
};

export const CurrentUserExcludedFromAutocomplete: Story = {
	beforeEach: () => mockDialogRequests(),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		const autocompleteButton = await body.findByRole("button", {
			name: "Search for user or group",
		});
		await userEvent.click(autocompleteButton);
		await userEvent.type(
			body.getByPlaceholderText("Search for user or group"),
			MockOrganizationMember.email,
		);

		await waitFor(() => {
			expect(body.getByText("No users or groups found")).toBeVisible();
			expect(
				body.queryByRole("option", {
					name: new RegExp(MockOrganizationMember.email, "i"),
				}),
			).not.toBeInTheDocument();
		});
	},
};

export const LoadingACL: Story = {
	beforeEach: () => mockDialogRequests({ aclPending: true }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(body.getByText("Loading chat sharing")).toBeInTheDocument();
		});
	},
};

export const ErrorACL: Story = {
	beforeEach: () =>
		mockDialogRequests({ aclError: new Error("Chat sharing is disabled") }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(body.getByText("Chat sharing is disabled")).toBeInTheDocument();
		});
	},
};

export const AddUser: Story = {
	decorators: [withToaster],
	beforeEach: () => mockDialogRequests(),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await addAutocompleteOption(body, {
			query: MockOrganizationMember2.email,
			option: new RegExp(MockOrganizationMember2.email, "i"),
		});

		await waitFor(() => {
			expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
				user_roles: { [MockUserMember.id]: "read" },
			});
		});
		await waitFor(() => {
			expect(body.getByText("Member added to chat.")).toBeVisible();
		});
	},
};

export const AddGroup: Story = {
	decorators: [withToaster],
	beforeEach: () => mockDialogRequests(),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await addAutocompleteOption(body, {
			query: MockGroup.name,
			option: new RegExp(MockGroup.display_name || MockGroup.name, "i"),
		});

		await waitFor(() => {
			expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
				group_roles: { [MockGroup.id]: "read" },
			});
		});
		await waitFor(() => {
			expect(body.getByText("Group added to chat.")).toBeVisible();
		});
	},
};

export const RemoveUser: Story = {
	decorators: [withToaster],
	beforeEach: () => mockDialogRequests({ acl: populatedACL }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(body.getAllByText(chatUser.username).length).toBeGreaterThan(0);
		});
		// Groups render before users, so the user row menu is the second one.
		const menuButtons = await body.findAllByRole("button", {
			name: "Open menu",
		});
		await userEvent.click(menuButtons[1]);
		const removeItem = await body.findByRole("menuitem", { name: "Remove" });
		await userEvent.click(removeItem);

		await waitFor(() => {
			expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
				user_roles: { [chatUser.id]: "" },
			});
		});
		await waitFor(() => {
			expect(body.getByText("Member removed from chat.")).toBeVisible();
		});
	},
};

export const RemoveGroup: Story = {
	decorators: [withToaster],
	beforeEach: () => mockDialogRequests({ acl: populatedACL }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(body.getAllByText(chatGroup.name).length).toBeGreaterThan(0);
		});
		// Groups render before users, so the group row menu is the first one.
		const menuButtons = await body.findAllByRole("button", {
			name: "Open menu",
		});
		await userEvent.click(menuButtons[0]);
		const removeItem = await body.findByRole("menuitem", { name: "Remove" });
		await userEvent.click(removeItem);

		await waitFor(() => {
			expect(API.experimental.updateChatACL).toHaveBeenCalledWith(chatId, {
				group_roles: { [chatGroup.id]: "" },
			});
		});
		await waitFor(() => {
			expect(body.getByText("Group removed from chat.")).toBeVisible();
		});
	},
};

export const MutationError: Story = {
	beforeEach: () =>
		mockDialogRequests({ updateError: new Error("No share permission") }),
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await addAutocompleteOption(body, {
			query: MockOrganizationMember2.email,
			option: new RegExp(MockOrganizationMember2.email, "i"),
		});
		await waitFor(() => {
			expect(body.getByText("No share permission")).toBeInTheDocument();
		});
	},
};

export const MutationErrorClearsAcrossMemberTypes: Story = {
	decorators: [withToaster],
	beforeEach: () => {
		const cleanup = mockLookupRequests();
		spyOn(API.experimental, "updateChatACL")
			.mockRejectedValueOnce(new Error("User add failed"))
			.mockResolvedValue(undefined);
		return cleanup;
	},
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await addAutocompleteOption(body, {
			query: MockOrganizationMember2.email,
			option: new RegExp(MockOrganizationMember2.email, "i"),
		});
		await waitFor(() => {
			expect(body.getByText("User add failed")).toBeInTheDocument();
		});

		await userEvent.click(
			body.getByRole("button", { name: "Clear selection" }),
		);
		await addAutocompleteOption(body, {
			query: MockGroup.name,
			option: new RegExp(MockGroup.display_name || MockGroup.name, "i"),
		});

		await waitFor(() => {
			expect(API.experimental.updateChatACL).toHaveBeenNthCalledWith(
				1,
				chatId,
				{
					user_roles: { [MockUserMember.id]: "read" },
				},
			);
			expect(API.experimental.updateChatACL).toHaveBeenNthCalledWith(
				2,
				chatId,
				{
					group_roles: { [MockGroup.id]: "read" },
				},
			);
		});
		await waitFor(() => {
			expect(body.queryByText("User add failed")).not.toBeInTheDocument();
			expect(body.getByText("Group added to chat.")).toBeVisible();
		});
	},
};

export const MutationErrorClearsWhenReopened: Story = {
	beforeEach: () => {
		const cleanup = mockLookupRequests();
		spyOn(API.experimental, "updateChatACL").mockRejectedValue(
			new Error("No share permission"),
		);
		return cleanup;
	},
	play: async ({ canvasElement }) => {
		const body = await openChatSharing(canvasElement);
		await addAutocompleteOption(body, {
			query: MockOrganizationMember2.email,
			option: new RegExp(MockOrganizationMember2.email, "i"),
		});
		await waitFor(() => {
			expect(body.getByText("No share permission")).toBeInTheDocument();
		});

		await closeChatSharing(canvasElement);
		const reopenedBody = await openChatSharing(canvasElement);
		await waitFor(() => {
			expect(
				reopenedBody.queryByText("No share permission"),
			).not.toBeInTheDocument();
		});
	},
};
