import type { Meta, StoryObj } from "@storybook/react-vite";
import { chatProjectMemoriesKey } from "#/api/queries/chatProjectsKeys";
import {
	MockChatProject,
	MockChatProjectMemory,
	MockChatProjectMemory2,
} from "#/testHelpers/entities";
import { ProjectMemorySection } from "./ProjectMemorySection";

const meta = {
	title: "pages/AgentsPage/ProjectMemorySection",
	component: ProjectMemorySection,
	args: { projectId: MockChatProject.id },
} satisfies Meta<typeof ProjectMemorySection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {
	parameters: {
		queries: [{ key: chatProjectMemoriesKey(MockChatProject.id), data: [] }],
	},
};

export const Populated: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectMemoriesKey(MockChatProject.id),
				data: [MockChatProjectMemory, MockChatProjectMemory2],
			},
		],
	},
};
