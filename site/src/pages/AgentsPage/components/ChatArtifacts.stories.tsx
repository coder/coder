import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ChatArtifacts } from "./ChatArtifacts";
import type { ChatArtifact } from "./ChatConversation/messageHelpers";

const mockArtifacts: ChatArtifact[] = [
	{
		key: "6:1",
		file: {
			type: "file",
			media_type: "image/png",
			name: "dashboard-screenshot.png",
			file_id: "screenshot-file",
		},
		href: "/api/v2/chats/files/screenshot-file",
	},
	{
		key: "5:2",
		file: {
			type: "file",
			media_type: "text/csv",
			name: "latency-report.csv",
			file_id: "report-file",
		},
		href: "/api/v2/chats/files/report-file",
	},
	{
		key: "4:1",
		file: {
			type: "file",
			media_type: "text/markdown",
			name: "PLAN.md",
			file_id: "plan-file",
		},
		href: "/api/v2/chats/files/plan-file",
	},
	{
		key: "3:1",
		file: {
			type: "file",
			media_type: "application/octet-stream",
			file_id: "unnamed-file",
		},
		href: "/api/v2/chats/files/unnamed-file",
	},
	{
		key: "2:1",
		file: {
			type: "file",
			media_type: "application/pdf",
			name: "quarterly-infrastructure-cost-breakdown-for-every-workspace-template.pdf",
			file_id: "long-name-file",
		},
		href: "/api/v2/chats/files/long-name-file",
	},
];

const meta: Meta<typeof ChatArtifacts> = {
	title: "pages/AgentsPage/ChatArtifacts",
	component: ChatArtifacts,
	decorators: [
		(Story) => (
			<div className="w-90 p-4">
				<Story />
			</div>
		),
	],
	args: {
		artifacts: mockArtifacts,
		evictedFileIds: new Set(),
		hasMoreMessages: false,
		isFetchingMoreMessages: false,
		isHydratingMessages: false,
		hasFetchMoreError: false,
		onFetchMoreMessages: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatArtifacts>;

export const WithArtifacts: Story = {};

export const Empty: Story = {
	args: { artifacts: [] },
};

export const HydratingMessages: Story = {
	args: { artifacts: [], isHydratingMessages: true },
};

export const EmptyWithEarlierMessages: Story = {
	args: { artifacts: [], hasMoreMessages: true },
};

export const WithEarlierMessages: Story = {
	args: { hasMoreMessages: true },
};

export const LoadingEarlierMessages: Story = {
	args: { hasMoreMessages: true, isFetchingMoreMessages: true },
};

export const LoadEarlierMessagesFailed: Story = {
	args: { hasMoreMessages: true, hasFetchMoreError: true },
};

export const ExpiredArtifact: Story = {
	args: { evictedFileIds: new Set(["report-file"]) },
};
