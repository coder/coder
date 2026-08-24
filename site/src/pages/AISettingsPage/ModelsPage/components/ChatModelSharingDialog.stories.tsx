import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockGroup,
	MockGroup2,
	MockOrganizationMember,
	MockOrganizationMember2,
} from "#/testHelpers/entities";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import { mockGPT5 } from "../testFixtures";
import { ChatModelSharingDialog } from "./ChatModelSharingDialog";

type MockACL = {
	user_roles: Record<string, "read">;
	group_roles: Record<string, "read">;
};

const emptyACL: MockACL = { user_roles: {}, group_roles: {} };
const populatedACL: MockACL = {
	user_roles: { [MockOrganizationMember2.user_id]: "read" as const },
	group_roles: { [MockGroup.id]: "read" as const },
};

const mockPrincipalRequests = () => {
	spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
		members: [MockOrganizationMember, MockOrganizationMember2],
		count: 2,
	});
	spyOn(API, "getGroupsByOrganization").mockResolvedValue([
		MockGroup,
		MockGroup2,
	]);
};

const mockRequests = ({
	acl = emptyACL,
	aclError,
	aclPending = false,
	updateError,
}: {
	acl?: MockACL;
	aclError?: Error;
	aclPending?: boolean;
	updateError?: Error;
} = {}) => {
	mockPrincipalRequests();
	if (aclPending) {
		spyOn(API.experimental, "getChatModelACL").mockReturnValue(
			new Promise(() => undefined),
		);
	} else if (aclError) {
		spyOn(API.experimental, "getChatModelACL").mockRejectedValue(aclError);
	} else {
		spyOn(API.experimental, "getChatModelACL").mockResolvedValue(acl);
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

const refreshedACL: MockACL = {
	user_roles: { [MockOrganizationMember.user_id]: "read" },
	group_roles: { [MockGroup2.id]: "read" },
};
let currentServerACL = populatedACL;

const ReopenableSharingDialog = () => {
	const [open, setOpen] = useState(true);
	return (
		<>
			<button type="button" onClick={() => setOpen(true)}>
				Open sharing
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
			await body.findByText("No shared members or groups yet"),
		).toBeInTheDocument();
		expect(body.getByRole("button", { name: "Save sharing" })).toBeDisabled();
	},
};

export const Loading: Story = {
	beforeEach: () => mockRequests({ aclPending: true }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(await body.findByRole("status")).toHaveTextContent(
			"Loading model sharing",
		);
	},
};

export const LoadError: Story = {
	beforeEach: () => mockRequests({ aclError: new Error("Unable to load ACL") }),
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(await body.findByText("Unable to load ACL")).toBeInTheDocument();
		expect(body.getByRole("button", { name: "Save sharing" })).toBeDisabled();
	},
};

export const AddUser: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await addAutocompleteOption(
			body,
			MockOrganizationMember2.email,
			new RegExp(MockOrganizationMember2.email, "i"),
		);

		expect(
			body.getByRole("row", {
				name: new RegExp(MockOrganizationMember2.username, "i"),
			}),
		).toBeVisible();
		const saveButton = body.getByRole("button", { name: "Save sharing" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() =>
			expect(API.experimental.updateChatModelACL).toHaveBeenCalledTimes(1),
		);
		expect(API.experimental.updateChatModelACL).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			mockGPT5.id,
			{ user_roles: { [MockOrganizationMember2.user_id]: "read" } },
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
		const saveButton = body.getByRole("button", { name: "Save sharing" });
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
				name: new RegExp(MockOrganizationMember2.email, "i"),
			}),
		).not.toBeInTheDocument();
		expect(
			body.queryByRole("option", {
				name: new RegExp(MockGroup.display_name || MockGroup.name, "i"),
			}),
		).not.toBeInTheDocument();
		await waitFor(() => {
			expect(
				body.getByRole("option", {
					name: new RegExp(MockOrganizationMember.email, "i"),
				}),
			).toBeVisible();
			expect(
				body.getByRole("option", {
					name: new RegExp(MockGroup2.display_name || MockGroup2.name, "i"),
				}),
			).toBeVisible();
		});
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
				name: `Remove ${MockOrganizationMember2.username}`,
			}),
		);
		await userEvent.click(body.getByRole("button", { name: "Save sharing" }));

		await waitFor(() =>
			expect(API.experimental.updateChatModelACL).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				mockGPT5.id,
				{
					user_roles: { [MockOrganizationMember2.user_id]: "" },
					group_roles: { [MockGroup.id]: "" },
				},
			),
		);
		expect(args.onOpenChange).toHaveBeenCalledWith(false);
	},
};

export const ReopenUsesFreshACL: Story = {
	render: () => <ReopenableSharingDialog />,
	beforeEach: () => {
		mockPrincipalRequests();
		currentServerACL = populatedACL;
		spyOn(API.experimental, "getChatModelACL").mockImplementation(
			async () => currentServerACL,
		);
		spyOn(API.experimental, "updateChatModelACL").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		expect(
			await body.findByRole("row", {
				name: new RegExp(MockOrganizationMember2.username, "i"),
			}),
		).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(
				body.queryByRole("dialog", { name: "Share model" }),
			).not.toBeInTheDocument(),
		);

		currentServerACL = refreshedACL;
		await userEvent.click(body.getByRole("button", { name: "Open sharing" }));

		expect(
			await body.findByRole("row", {
				name: new RegExp(MockOrganizationMember.username, "i"),
			}),
		).toBeInTheDocument();
		expect(
			body.getByRole("row", {
				name: new RegExp(MockGroup2.display_name || MockGroup2.name, "i"),
			}),
		).toBeInTheDocument();
		expect(
			body.queryByRole("row", {
				name: new RegExp(MockOrganizationMember2.username, "i"),
			}),
		).not.toBeInTheDocument();
		expect(API.experimental.getChatModelACL).toHaveBeenCalled();
	},
};

export const SaveErrorKeepsEditorOpen: Story = {
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
		await userEvent.click(body.getByRole("button", { name: "Save sharing" }));

		const alert = await body.findByRole("alert");
		expect(alert).toHaveTextContent("Unable to save ACL");
		expect(body.getByRole("dialog", { name: "Share model" })).toHaveAttribute(
			"data-state",
			"open",
		);
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
