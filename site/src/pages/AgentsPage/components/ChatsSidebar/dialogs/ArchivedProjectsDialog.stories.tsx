import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "react-query";
import { fn, spyOn, within } from "storybook/test";
import { API } from "#/api/api";
import {
	archivedChatProjects,
	archivedChatProjectsKey,
} from "#/api/queries/chatProjects";
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

/** A shared project the viewer can't unarchive offers only Open. */
export const Default: Story = {
	parameters: {
		queries: [
			{
				key: archivedChatProjectsKey,
				data: [
					MockArchivedChatProject,
					{
						...MockArchivedChatProject,
						id: "billing",
						name: "Billing migration with a name long enough to truncate in the row",
						permissions: {
							update: false,
							delete: false,
							share: false,
							archive: false,
						},
					},
				],
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
				data: [
					MockArchivedChatProject,
					{
						...MockArchivedChatProject,
						id: "launch-2",
						organization_id: MockOrganization2.id,
					},
				],
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
		await within(document.body).findByRole("button", { name: "Retry" });
	},
};
