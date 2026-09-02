import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModelACLAvailable,
	MockDefaultOrganization,
	MockEveryoneGroup,
	MockGroup,
	MockGroup2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import { mockGPT5 } from "../testFixtures";
import { ChatModelSharingDialog } from "./ChatModelSharingDialog";

type MockACL = TypesGen.ChatModelACL;

const emptyACL: MockACL = { users: [], groups: [] };
const populatedACL: MockACL = {
	users: [{ ...MockUserMember, role: "read" }],
	groups: [{ ...MockGroup, role: "read" }],
};
const everyoneACL: MockACL = {
	users: [],
	groups: [{ ...MockEveryoneGroup, role: "read" }],
};
const refreshedACL: MockACL = {
	users: [{ ...MockUserOwner, role: "read" }],
	groups: [{ ...MockGroup2, role: "read" }],
};

const mockLegacyPrincipalRequests = () => {
	spyOn(API, "getOrganizationPaginatedMembers").mockRejectedValue(
		new Error("Legacy organization member discovery must not be called"),
	);
	spyOn(API, "getGroupsByOrganization").mockRejectedValue(
		new Error("Legacy organization group discovery must not be called"),
	);
};

const mockRequests = ({
	acl = emptyACL,
	aclError,
	aclPending = false,
	availableError,
	updateError,
}: {
	acl?: MockACL;
	aclError?: Error;
	aclPending?: boolean;
	availableError?: Error;
	updateError?: Error;
} = {}) => {
	mockLegacyPrincipalRequests();
	if (aclPending) {
		spyOn(API.experimental, "getChatModelACL").mockReturnValue(
			new Promise(() => undefined),
		);
	} else if (aclError) {
		spyOn(API.experimental, "getChatModelACL").mockRejectedValue(aclError);
	} else {
		spyOn(API.experimental, "getChatModelACL").mockResolvedValue(acl);
	}
	if (availableError) {
		spyOn(API.experimental, "getChatModelACLAvailable").mockRejectedValue(
			availableError,
		);
	} else {
		spyOn(API.experimental, "getChatModelACLAvailable").mockResolvedValue(
			MockChatModelACLAvailable,
		);
	}
	if (updateError) {
		spyOn(API.experimental, "updateChatModelACL").mockRejectedValue(
			updateError,
		);
	} else {
		spyOn(API.experimental, "updateChatModelACL").mockResolvedValue();
	}
};

const addAutocompleteOption = async (
	body: ReturnType<typeof within>,
	query: string,
	option: string | RegExp,
) => {
	await userEvent.click(
		await body.findByRole("button", { name: "Search for user or group" }),
	);
	await userEvent.type(
		body.getByPlaceholderText("Search for user or group"),
		query,
	);
	await userEvent.click(await body.findByRole("option", { name: option }));
	await userEvent.click(body.getByRole("button", { name: "Add member" }));
};

let currentServerACL = populatedACL;

const ReopenableSharingDialog = () => {
	const [open, setOpen] = useState(true);
	return (
		<>
			<button type="button" onClick={() => setOpen(true)}>
				Open permissions
			</button>
			<ChatModelSharingDialog
				open={open}
				onOpenChange={setOpen}
				organizationId={MockDefaultOrganization.id}
				modelId={mockGPT5.id}
				modelName={mockGPT5.display_name}
			/>
		</>
	);
};

const meta: Meta<typeof ChatModelSharingDialog> = {
	title: "pages/AISettingsPage/ModelsPage/ChatModelSharingDialog",
	component: ChatModelSharingDialog,
	decorators: [withDashboardProvider, withToaster],
	args: {
		open: true,
		onOpenChange: fn(),
		organizationId: MockDefaultOrganization.id,
		modelId: mockGPT5.id,
		modelName: mockGPT5.display_name,
	},
};

export default meta;
type Story = StoryObj<typeof ChatModelSharingDialog>;

export const EmptyACL: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(
			await body.findByRole("dialog", { name: "Model permissions" }),
		).toHaveAttribute("data-state", "open");
		expect(
			body.getByText(
				`Manage which organization members and groups can use ${mockGPT5.display_name}.`,
			),
		).toBeInTheDocument();
		expect(
			await body.findByText("No members or groups have permission yet"),
		).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: "Save permissions" }),
		).toBeDisabled();
		expect(API.getOrganizationPaginatedMembers).not.toHaveBeenCalled();
		expect(API.getGroupsByOrganization).not.toHaveBeenCalled();
	},
};

export const Loading: Story = {
	beforeEach: () => mockRequests({ aclPending: true }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(await body.findByRole("status")).toHaveTextContent(
			"Loading model permissions",
		);
	},
};

export const InitialACLFailureIsBlocking: Story = {
	beforeEach: () => mockRequests({ aclError: new Error("Unable to load ACL") }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(await body.findByText("Unable to load ACL")).toBeInTheDocument();
		expect(
			body.getByRole("button", { name: "Save permissions" }),
		).toBeDisabled();
		expect(
			body.queryByRole("button", { name: "Search for user or group" }),
		).not.toBeInTheDocument();
		expect(
			body.queryByRole("table", {
				name: "Model permissions for members and groups",
			}),
		).not.toBeInTheDocument();
	},
};

export const HydratedPrincipalsRenderWithoutIDs: Story = {
	beforeEach: () => mockRequests({ acl: populatedACL }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(
			await body.findByRole("row", {
				name: new RegExp(MockUserMember.username, "i"),
			}),
		).toBeInTheDocument();
		expect(
			body.getByRole("row", {
				name: new RegExp(MockGroup.display_name || MockGroup.name, "i"),
			}),
		).toBeInTheDocument();
		expect(body.queryByText(MockUserMember.id)).not.toBeInTheDocument();
		expect(body.queryByText(MockGroup.id)).not.toBeInTheDocument();
		expect(API.getOrganizationPaginatedMembers).not.toHaveBeenCalled();
		expect(API.getGroupsByOrganization).not.toHaveBeenCalled();
	},
};

export const EveryoneGroup: Story = {
	beforeEach: () => mockRequests({ acl: everyoneACL }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const everyoneRow = await body.findByRole("row", { name: /Everyone/i });
		expect(everyoneRow).toHaveTextContent("Everyone");
		expect(everyoneRow).toHaveTextContent("All users");
	},
};

export const AddUser: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await addAutocompleteOption(
			body,
			MockUserMember.email,
			new RegExp(MockUserMember.email, "i"),
		);

		const userRow = body.getByRole("row", {
			name: new RegExp(MockUserMember.username, "i"),
		});
		expect(userRow).toBeVisible();
		expect(userRow).toHaveTextContent("Use");
		const saveButton = body.getByRole("button", { name: "Save permissions" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() =>
			expect(API.experimental.updateChatModelACL).toHaveBeenCalledTimes(1),
		);
		expect(API.experimental.updateChatModelACL).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			mockGPT5.id,
			{ user_roles: { [MockUserMember.id]: "read" } },
		);
		expect(args.onOpenChange).toHaveBeenCalledWith(false);
	},
};

export const AddGroup: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await addAutocompleteOption(
			body,
			MockGroup2.name,
			new RegExp(MockGroup2.display_name || MockGroup2.name, "i"),
		);

		expect(
			body.getByRole("row", {
				name: new RegExp(MockGroup2.display_name || MockGroup2.name, "i"),
			}),
		).toBeVisible();
		const saveButton = body.getByRole("button", { name: "Save permissions" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() =>
			expect(API.experimental.updateChatModelACL).toHaveBeenCalledTimes(1),
		);
		expect(API.experimental.updateChatModelACL).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			mockGPT5.id,
			{ group_roles: { [MockGroup2.id]: "read" } },
		);
		expect(args.onOpenChange).toHaveBeenCalledWith(false);
	},
};

export const SelectedPrincipalsExcludedFromAutocomplete: Story = {
	beforeEach: () => mockRequests({ acl: populatedACL }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", {
				name: "Search for user or group",
			}),
		);

		expect(
			body.queryByRole("option", {
				name: new RegExp(MockUserMember.email, "i"),
			}),
		).not.toBeInTheDocument();
		expect(
			body.queryByRole("option", {
				name: new RegExp(MockGroup.display_name || MockGroup.name, "i"),
			}),
		).not.toBeInTheDocument();
		expect(
			await body.findByRole("option", {
				name: new RegExp(MockUserOwner.email, "i"),
			}),
		).toBeInTheDocument();
		expect(
			body.getByRole("option", {
				name: new RegExp(MockGroup2.display_name || MockGroup2.name, "i"),
			}),
		).toBeInTheDocument();
	},
};

export const SaveRemovalsAsSparseDelta: Story = {
	beforeEach: () => mockRequests({ acl: populatedACL }),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", {
				name: `Remove ${MockGroup.display_name}`,
			}),
		);
		await userEvent.click(
			body.getByRole("button", {
				name: `Remove ${MockUserMember.username}`,
			}),
		);
		await userEvent.click(
			body.getByRole("button", { name: "Save permissions" }),
		);

		await waitFor(() =>
			expect(API.experimental.updateChatModelACL).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				mockGPT5.id,
				{
					user_roles: { [MockUserMember.id]: "" },
					group_roles: { [MockGroup.id]: "" },
				},
			),
		);
		expect(args.onOpenChange).toHaveBeenCalledWith(false);
	},
};

export const CandidateDiscoveryFailureKeepsACLUsable: Story = {
	beforeEach: () =>
		mockRequests({
			acl: populatedACL,
			availableError: new Error("Unable to discover principals"),
		}),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const userRow = await body.findByRole("row", {
			name: new RegExp(MockUserMember.username, "i"),
		});
		await userEvent.click(
			body.getByRole("button", { name: "Search for user or group" }),
		);
		expect(await body.findByRole("alert")).toHaveTextContent(
			"Unable to discover principals",
		);
		expect(userRow).toBeInTheDocument();
		await userEvent.click(
			body.getByRole("button", { name: `Remove ${MockGroup.display_name}` }),
		);
		await userEvent.click(
			body.getByRole("button", { name: "Save permissions" }),
		);

		await waitFor(() =>
			expect(API.experimental.updateChatModelACL).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				mockGPT5.id,
				{ group_roles: { [MockGroup.id]: "" } },
			),
		);
	},
};

export const ReopenUsesFreshACL: Story = {
	render: () => <ReopenableSharingDialog />,
	beforeEach: () => {
		mockLegacyPrincipalRequests();
		currentServerACL = populatedACL;
		spyOn(API.experimental, "getChatModelACL").mockImplementation(
			async () => currentServerACL,
		);
		spyOn(API.experimental, "getChatModelACLAvailable").mockResolvedValue(
			MockChatModelACLAvailable,
		);
		spyOn(API.experimental, "updateChatModelACL").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(
			await body.findByRole("row", {
				name: new RegExp(MockUserMember.username, "i"),
			}),
		).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(
				body.queryByRole("dialog", { name: "Model permissions" }),
			).not.toBeInTheDocument(),
		);

		currentServerACL = refreshedACL;
		await userEvent.click(
			body.getByRole("button", { name: "Open permissions" }),
		);

		expect(
			await body.findByRole("row", {
				name: new RegExp(MockUserOwner.username, "i"),
			}),
		).toBeInTheDocument();
		expect(
			body.getByRole("row", {
				name: new RegExp(MockGroup2.display_name || MockGroup2.name, "i"),
			}),
		).toBeInTheDocument();
		expect(
			body.queryByRole("row", {
				name: new RegExp(MockUserMember.username, "i"),
			}),
		).not.toBeInTheDocument();
		expect(API.experimental.getChatModelACL).toHaveBeenCalledTimes(2);
	},
};

export const SaveErrorPreservesDraft: Story = {
	beforeEach: () =>
		mockRequests({
			acl: populatedACL,
			updateError: new Error("Unable to save ACL"),
		}),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", {
				name: `Remove ${MockGroup.display_name}`,
			}),
		);
		await userEvent.click(
			body.getByRole("button", { name: "Save permissions" }),
		);

		const alert = await body.findByRole("alert");
		expect(alert).toHaveTextContent("Unable to save ACL");
		expect(
			body.getByRole("dialog", { name: "Model permissions" }),
		).toHaveAttribute("data-state", "open");
		expect(
			body.queryByRole("row", {
				name: new RegExp(MockGroup.display_name || MockGroup.name, "i"),
			}),
		).not.toBeInTheDocument();
		expect(
			body.getByRole("button", { name: "Save permissions" }),
		).toBeEnabled();
		expect(args.onOpenChange).not.toHaveBeenCalledWith(false);
	},
};

export const CancelDiscardsDraft: Story = {
	beforeEach: () => mockRequests({ acl: populatedACL }),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", {
				name: `Remove ${MockGroup.display_name}`,
			}),
		);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		expect(API.experimental.updateChatModelACL).not.toHaveBeenCalled();
		expect(args.onOpenChange).toHaveBeenCalledWith(false);
	},
};
