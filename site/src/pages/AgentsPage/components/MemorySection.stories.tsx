import type { Meta, StoryObj } from "@storybook/react-vite";
import { chatProjectMemoriesKey } from "#/api/queries/chatProjectsKeys";
import { chatUserMemoriesKey } from "#/api/queries/chatUserMemories";
import {
	MockChatProject,
	MockChatProjectMemory,
	MockChatProjectMemory2,
	MockChatUserMemory,
	MockChatUserMemory2,
	MockDefaultOrganization,
} from "#/testHelpers/entities";
import { MemorySection } from "./MemorySection";

const meta = {
	title: "pages/AgentsPage/MemorySection",
	component: MemorySection,
} satisfies Meta<typeof MemorySection>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ProjectEmpty: Story = {
	args: { scope: { kind: "project", projectId: MockChatProject.id } },
	parameters: {
		queries: [{ key: chatProjectMemoriesKey(MockChatProject.id), data: [] }],
	},
};

export const ProjectPopulated: Story = {
	args: { scope: { kind: "project", projectId: MockChatProject.id } },
	parameters: {
		queries: [
			{
				key: chatProjectMemoriesKey(MockChatProject.id),
				data: [MockChatProjectMemory, MockChatProjectMemory2],
			},
		],
	},
};

export const PersonalEmpty: Story = {
	args: {
		scope: { kind: "personal", organizationId: MockDefaultOrganization.id },
	},
	parameters: {
		queries: [
			{ key: chatUserMemoriesKey(MockDefaultOrganization.id), data: [] },
		],
	},
};

export const PersonalPopulated: Story = {
	args: {
		scope: { kind: "personal", organizationId: MockDefaultOrganization.id },
	},
	parameters: {
		queries: [
			{
				key: chatUserMemoriesKey(MockDefaultOrganization.id),
				data: [MockChatUserMemory, MockChatUserMemory2],
			},
		],
	},
};
