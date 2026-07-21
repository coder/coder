import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type { ChatModelConfigACL } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockGroup,
	MockGroup2,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserMember,
} from "#/testHelpers/entities";
import { ResourceSharingPageView } from "./ResourceSharingPageView";

const organizationId = MockDefaultOrganization.id;
const user = {
	id: MockUserMember.id,
	username: MockUserMember.username,
	name: MockUserMember.name,
	avatar_url: MockUserMember.avatar_url,
	role: "read" as const,
};
const group = { ...MockGroup, role: "read" as const };
const everyoneGroup = {
	...MockGroup2,
	id: organizationId,
	organization_id: organizationId,
	name: "Everyone",
	display_name: "Everyone",
};
const emptyACL: ChatModelConfigACL = { users: [], groups: [] };
const populatedACL: ChatModelConfigACL = { users: [user], groups: [group] };

const baseArgs = {
	resourceName: "GPT-5",
	resourceTypeLabel: "model",
	backPath: "/ai/settings/models/model-gpt5",
	search: `?organization=${MockDefaultOrganization.name}`,
	organizationId,
	acl: emptyACL,
	isLoading: false,
	error: undefined,
	mutationError: undefined,
	canShare: true,
	isMutating: false,
	onAddUser: fn(async () => undefined),
	onAddGroup: fn(async () => undefined),
	onRemoveUser: fn(async () => undefined),
	onRemoveGroup: fn(async () => undefined),
};

const mockCandidates = () => {
	spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
		members: [MockOrganizationMember, MockOrganizationMember2],
		count: 2,
	});
	spyOn(API, "getGroupsByOrganization").mockResolvedValue([
		everyoneGroup,
		MockGroup,
		MockGroup2,
	]);
};

const selectCandidate = async (
	canvasElement: HTMLElement,
	query: string,
	optionName: RegExp,
) => {
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(
		within(canvasElement).getByRole("button", {
			name: "Search for user or group",
		}),
	);
	await userEvent.type(
		body.getByPlaceholderText("Search for user or group"),
		query,
	);
	await userEvent.click(await body.findByRole("option", { name: optionName }));
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Add member" }),
	);
};

const meta: Meta<typeof ResourceSharingPageView> = {
	title: "pages/AISettingsPage/ResourceSharingPageView",
	component: ResourceSharingPageView,
	args: baseArgs,
};

export default meta;
type Story = StoryObj<typeof ResourceSharingPageView>;

export const EmptyACL: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("No users or groups have access"),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Add member" }),
		).toBeDisabled();
	},
};

export const UserGrant: Story = {
	beforeEach: mockCandidates,
	play: async ({ canvasElement, args }) => {
		await selectCandidate(
			canvasElement,
			MockOrganizationMember2.email,
			new RegExp(MockOrganizationMember2.email, "i"),
		);
		await waitFor(() => {
			expect(args.onAddUser).toHaveBeenCalledWith(
				MockOrganizationMember2.user_id,
			);
		});
	},
};

export const GroupGrant: Story = {
	beforeEach: mockCandidates,
	play: async ({ canvasElement, args }) => {
		await selectCandidate(canvasElement, MockGroup2.name, /developer/i);
		await waitFor(() => {
			expect(args.onAddGroup).toHaveBeenCalledWith(MockGroup2.id);
		});
	},
};

export const EveryoneGroupGrant: Story = {
	beforeEach: mockCandidates,
	play: async ({ canvasElement, args }) => {
		await selectCandidate(canvasElement, "Everyone", /Everyone All users/i);
		await waitFor(() => {
			expect(args.onAddGroup).toHaveBeenCalledWith(organizationId);
		});
	},
};

export const Removal: Story = {
	args: { acl: populatedACL },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: `Manage ${user.username}` }),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Remove" }),
		);
		await waitFor(() => {
			expect(args.onRemoveUser).toHaveBeenCalledWith(user.id);
		});
	},
};

export const ReadOnly: Story = {
	args: { acl: populatedACL, canShare: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/you do not have permission to change them/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Add member" }),
		).not.toBeInTheDocument();
		await expect(canvas.queryByLabelText(/Manage/)).not.toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: { acl: undefined, isLoading: true },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText("Loading sharing settings"),
		).toBeVisible();
	},
};

export const APIError: Story = {
	args: { acl: undefined, error: new Error("Unable to load sharing settings") },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText("Unable to load sharing settings"),
		).toBeVisible();
	},
};

export const OrganizationRestrictedCandidates: Story = {
	beforeEach: mockCandidates,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("button", { name: "Search for user or group" }),
		);
		await waitFor(() => {
			expect(API.getOrganizationPaginatedMembers).toHaveBeenCalledWith(
				organizationId,
				{ limit: 0 },
			);
			expect(API.getGroupsByOrganization).toHaveBeenCalledWith(organizationId);
		});
		await userEvent.type(
			body.getByPlaceholderText("Search for user or group"),
			MockOrganizationMember.email,
		);
		await expect(
			body.getByRole("option", {
				name: new RegExp(MockOrganizationMember.email, "i"),
			}),
		).toBeVisible();
	},
};

const MutationFailureHarness = () => {
	const [mutationError, setMutationError] = useState<unknown>();
	return (
		<ResourceSharingPageView
			{...baseArgs}
			mutationError={mutationError}
			onAddUser={async () => {
				setMutationError(new Error("Failed to grant access"));
				throw new Error("Failed to grant access");
			}}
		/>
	);
};

export const MutationFailure: Story = {
	render: () => <MutationFailureHarness />,
	beforeEach: mockCandidates,
	play: async ({ canvasElement }) => {
		await selectCandidate(
			canvasElement,
			MockOrganizationMember2.email,
			new RegExp(MockOrganizationMember2.email, "i"),
		);
		await expect(
			within(canvasElement).findByText("Failed to grant access"),
		).resolves.toBeVisible();
	},
};
