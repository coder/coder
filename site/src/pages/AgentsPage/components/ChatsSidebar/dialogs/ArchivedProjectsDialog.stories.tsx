import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import { archivedChatProjectsKey } from "#/api/queries/chatProjects";
import {
	MockArchivedChatProject,
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
