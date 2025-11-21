import {
	createTimestamp,
	MockTaskWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceAppStatuses,
} from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceAppStatus } from "api/typesGenerated";
import { userEvent, within } from "storybook/test";
import { AppStatuses } from "./AppStatuses";

const meta: Meta<typeof AppStatuses> = {
	title: "pages/WorkspacePage/AppStatuses",
	component: AppStatuses,
	args: {
		referenceDate: new Date("2024-03-26T15:15:00Z"),
		agent: mockAgent(MockWorkspaceAppStatuses),
		workspace: MockTaskWorkspace,
	},
	decorators: [withProxyProvider()],
};

export default meta;

type Story = StoryObj<typeof AppStatuses>;

export const Default: Story = {};

// Add a story with a "Working" status as the latest
export const WorkingState: Story = {
	args: {
		agent: mockAgent([
			{
				// This is now the latest (15:05:15) and is "working"
				...MockWorkspaceAppStatus,
				id: "status-8",
				icon: "", // Let the component handle the spinner icon
				message: "Processing final checks...",
				created_at: createTimestamp(5, 15), // 15:05:15 (after referenceDate)
				uri: "",
				state: "working" as const,
			},
			...MockWorkspaceAppStatuses,
		]),
	},
};

export const IdleState: Story = {
	args: {
		agent: mockAgent([
			{
				...MockWorkspaceAppStatus,
				id: "status-8",
				icon: "",
				message: "Done for now",
				created_at: createTimestamp(5, 20),
				uri: "",
				state: "idle" as const,
			},
			...MockWorkspaceAppStatuses,
		]),
	},
};

export const NoMessage: Story = {
	args: {
		agent: mockAgent([
			{
				...MockWorkspaceAppStatus,
				id: "status-8",
				icon: "",
				message: "",
				created_at: createTimestamp(5, 20),
				uri: "",
				state: "idle" as const,
			},
			...MockWorkspaceAppStatuses,
		]),
	},
};

export const LongStatusText: Story = {
	args: {
		agent: mockAgent([
			{
				// This is now the latest (15:05:15) and is "working"
				...MockWorkspaceAppStatus,
				id: "status-8",
				icon: "", // Let the component handle the spinner icon
				message:
					"Processing final checks with a very long message that exceeds the usual length to test how the component handles overflow and truncation in the UI. This should be long enough to ensure it wraps correctly and doesn't break the layout.",
				created_at: createTimestamp(5, 15), // 15:05:15 (after referenceDate)
				uri: "",
				state: "complete" as const,
			},
			...MockWorkspaceAppStatuses,
		]),
	},
};

export const SingleStatus: Story = {
	args: {
		agent: mockAgent([
			{
				...MockWorkspaceAppStatus,
				id: "status-1",
				icon: "",
				message: "Initial setup complete.",
				created_at: createTimestamp(5, 10), // 15:05:10 (after referenceDate)
				uri: "",
				state: "complete" as const,
			},
		]),
	},
};

export const MultipleStatuses: Story = {
	args: {
		agent: mockAgent([
			{
				...MockWorkspaceAppStatus,
				id: "status-1",
				icon: "",
				message: "Initial setup complete.",
				created_at: createTimestamp(5, 10), // 15:05:10 (after referenceDate)
				uri: "",
				state: "complete" as const,
			},
			{
				...MockWorkspaceAppStatus,
				id: "status-2",
				icon: "",
				message: "Working...",
				created_at: createTimestamp(5, 0), // 15:05:00 (after referenceDate)
				uri: "",
				state: "working" as const,
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const submitButton = canvas.getByRole("button");
		await userEvent.click(submitButton);
		await canvas.findByText(/working/i);
	},
};

export const NoTaskWorkspace: Story = {
	args: {
		agent: mockAgent([
			{
				...MockWorkspaceAppStatus,
				id: "status-9",
				icon: "",
				message: "status updated via curl",
				created_at: createTimestamp(5, 15),
				uri: "",
				state: "complete" as const,
			},
			...MockWorkspaceAppStatuses,
		]),
		workspace: MockWorkspace,
	},
};

function mockAgent(statuses: WorkspaceAppStatus[]) {
	return {
		...MockWorkspaceAgent,
		apps: [
			{
				...MockWorkspaceApp,
				statuses,
			},
		],
	};
}
