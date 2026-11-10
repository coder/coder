import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { archivedChatProjectsKey } from "#/api/queries/chatProjects";
import { MockChatProject } from "#/testHelpers/entities";
import { ArchivedProjectsDialog } from "./ArchivedProjectsDialog";

const meta = {
	title: "pages/AgentsPage/ArchivedProjectsDialog",
	component: ArchivedProjectsDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		locationSearch: "",
	},
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
					{ ...MockChatProject, archived_at: "2026-11-10T12:00:00Z" },
					{
						...MockChatProject,
						id: "billing",
						name: "Billing migration",
						archived_at: "2026-11-09T12:00:00Z",
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
