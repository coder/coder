import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "react-query";
import { fn, screen, spyOn, userEvent, waitFor } from "storybook/test";
import { API } from "#/api/api";
import {
	archivedChatProjects,
	archivedChatProjectsKey,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import {
	MockArchivedChatProject,
	MockDefaultOrganization,
	MockOrganization2,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { ArchivedProjectsDialog } from "./ArchivedProjectsDialog";

const meta = {
	title: "pages/AgentsPage/ArchivedProjectsDialog",
	component: ArchivedProjectsDialog,
	decorators: [withAuthProvider],
	args: {
		open: true,
		onOpenChange: fn(),
		organizations: [MockDefaultOrganization, MockOrganization2],
		locationSearch: "",
	},
	parameters: { user: MockUserOwner },
} satisfies Meta<typeof ArchivedProjectsDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

const mockSharedBillingProject: ChatProject = {
	...MockArchivedChatProject,
	id: "billing",
	name: "Billing migration with a name long enough to truncate in the row",
	permissions: { update: false, delete: false, share: false, archive: false },
};

const mockBillingProject: ChatProject = {
	...MockArchivedChatProject,
	id: "billing",
	name: "Billing",
};

const mockOtherOrganizationLaunchProject: ChatProject = {
	...MockArchivedChatProject,
	id: "launch-2",
	organization_id: MockOrganization2.id,
};

const mockLongNameProject: ChatProject = {
	...MockArchivedChatProject,
	name: "Launch plan for the quarterly release with a name long enough to truncate",
};

const mockOtherOrganizationLongNameProject: ChatProject = {
	...mockOtherOrganizationLaunchProject,
	name: mockLongNameProject.name,
};

/** A shared project the viewer can't unarchive offers only Open. */
export const Default: Story = {
	parameters: {
		queries: [
			{
				key: archivedChatProjectsKey,
				data: [MockArchivedChatProject, mockSharedBillingProject],
			},
		],
	},
};

export const Empty: Story = {
	parameters: {
		queries: [{ key: archivedChatProjectsKey, data: [] }],
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockRejectedValue(
			mockApiError({ message: "Failed to list archived projects." }),
		);
	},
};

/** Same-named projects in two organizations show the organization. */
export const SameNameInTwoOrganizations: Story = {
	parameters: {
		queries: [
			{
				key: archivedChatProjectsKey,
				data: [MockArchivedChatProject, mockOtherOrganizationLaunchProject],
			},
		],
	},
};

/** Long names and organization labels both truncate beside the actions. */
export const LongNamesInTwoOrganizations: Story = {
	args: {
		organizations: [
			{
				...MockDefaultOrganization,
				display_name: "Platform engineering and developer experience",
			},
			{
				...MockOrganization2,
				display_name: "Customer success and solutions engineering",
			},
		],
	},
	parameters: {
		queries: [
			{
				key: archivedChatProjectsKey,
				data: [mockLongNameProject, mockOtherOrganizationLongNameProject],
			},
		],
	},
};

/** A failed refresh keeps the cached rows below the error. */
export const RefetchError: Story = {
	parameters: {
		queries: [
			{ key: archivedChatProjectsKey, data: [MockArchivedChatProject] },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockRejectedValue(
			mockApiError({ message: "Failed to refresh archived projects." }),
		);
	},
	render: function RefetchErrorRender(args) {
		// Seeded data never goes stale in Storybook, so a stale-on-mount
		// observer refetches it and fails on top of the cached rows.
		useQuery({ ...archivedChatProjects(), staleTime: 0, retry: false });
		return <ArchivedProjectsDialog {...args} />;
	},
	play: async () => {
		await screen.findByRole("button", { name: "Retry" });
	},
};

/** The Unarchive button spins and disables while its request is pending. */
export const UnarchivePending: Story = {
	parameters: {
		queries: [
			{ key: archivedChatProjectsKey, data: [MockArchivedChatProject] },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "unarchiveChatProject").mockReturnValue(
			new Promise(() => {}),
		);
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", {
				name: `Unarchive ${MockArchivedChatProject.name}`,
			}),
		);
	},
};

/** A rejected unarchive keeps the row and re-enables its button. */
export const UnarchiveError: Story = {
	parameters: {
		queries: [
			{ key: archivedChatProjectsKey, data: [MockArchivedChatProject] },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "unarchiveChatProject").mockRejectedValue(
			mockApiError({ message: "Failed to unarchive project." }),
		);
		// The failed request still refreshes the list.
		spyOn(API.experimental, "getChatProjects").mockResolvedValue([
			MockArchivedChatProject,
		]);
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", {
				name: `Unarchive ${MockArchivedChatProject.name}`,
			}),
		);
	},
};

/** Unarchiving with the keyboard hands focus to the remaining row's action. */
export const UnarchiveWithKeyboard: Story = {
	parameters: {
		queries: [
			{
				key: archivedChatProjectsKey,
				data: [MockArchivedChatProject, mockBillingProject],
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "unarchiveChatProject").mockResolvedValue({
			...MockArchivedChatProject,
			archived_at: undefined,
		});
		spyOn(API.experimental, "getChatProjects").mockImplementation(
			async (params) => (params?.archived ? [mockBillingProject] : []),
		);
	},
	play: async () => {
		const unarchive = await screen.findByRole("button", {
			name: `Unarchive ${MockArchivedChatProject.name}`,
		});
		unarchive.focus();
		await userEvent.keyboard("{Enter}");
		await waitFor(() => {
			if (unarchive.isConnected) {
				throw new Error("The unarchived row is still listed.");
			}
		});
	},
};
