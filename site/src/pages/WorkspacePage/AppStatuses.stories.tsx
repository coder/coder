import {
	MockWorkspaceAgent,
	MockWorkspace,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
} from "testHelpers/entities";
import { AppStatuses } from "./AppStatuses";
import type { Meta, StoryObj } from "@storybook/react";
import type { WorkspaceAppStatus as APIWorkspaceAppStatus } from "api/typesGenerated";
import { MockProxyLatencies } from "testHelpers/entities";
import { getPreferredProxy, ProxyContext } from "contexts/ProxyContext";

const meta: Meta<typeof AppStatuses> = {
	title: "pages/WorkspacePage/AppStatuses",
	component: AppStatuses,
	// Add decorator for ProxyContext
	decorators: [
		(Story) => (
			<ProxyContext.Provider
				value={{
					proxyLatencies: MockProxyLatencies,
					proxy: getPreferredProxy([], undefined),
					proxies: [],
					isLoading: false,
					isFetched: true,
					clearProxy: () => {
						return;
					},
					setProxy: () => {
						return;
					},
					refetchProxyLatencies: (): Date => {
						return new Date();
					},
				}}
			>
				<Story />
			</ProxyContext.Provider>
		),
	],
};

export default meta;

type Story = StoryObj<typeof AppStatuses>;

// Helper function to create timestamps easily
const createTimestamp = (
	minuteOffset: number,
	secondOffset: number,
): string => {
	const baseDate = new Date("2024-03-26T15:00:00Z");
	baseDate.setMinutes(baseDate.getMinutes() + minuteOffset);
	baseDate.setSeconds(baseDate.getSeconds() + secondOffset);
	return baseDate.toISOString();
};

// Define a fixed reference date for Storybook, slightly after the last status
const storyReferenceDate = new Date("2024-03-26T15:15:00Z"); // 15 minutes after base

export const Default: Story = {
	args: {
		workspace: MockWorkspace,
		agents: [MockWorkspaceAgent],
		apps: [
			{
				...MockWorkspaceApp,
				statuses: [
					{
						// This is the latest status chronologically (15:04:38)
						...MockWorkspaceAppStatus,
						id: "status-7",
						icon: "/emojis/1f4dd.png", // 📝
						message: "Creating PR with gh CLI",
						created_at: createTimestamp(4, 38), // 15:04:38
						uri: "https://github.com/coder/coder/pull/5678",
						state: "complete" as const,
					},
					{
						// (15:03:56)
						...MockWorkspaceAppStatus,
						id: "status-6",
						icon: "/emojis/1f680.png", // 🚀
						message: "Pushing branch to remote",
						created_at: createTimestamp(3, 56), // 15:03:56
						uri: "",
						state: "complete" as const,
					},
					{
						// (15:02:29)
						...MockWorkspaceAppStatus,
						id: "status-5",
						icon: "/emojis/1f527.png", // 🔧
						message: "Configuring git identity",
						created_at: createTimestamp(2, 29), // 15:02:29
						uri: "",
						state: "complete" as const,
					},
					{
						// (15:02:04)
						...MockWorkspaceAppStatus,
						id: "status-4",
						icon: "/emojis/1f4be.png", // 💾
						message: "Committing changes",
						created_at: createTimestamp(2, 4), // 15:02:04
						uri: "",
						state: "complete" as const,
					},
					{
						// (15:01:44)
						...MockWorkspaceAppStatus,
						id: "status-3",
						icon: "/emojis/2795.png", // +
						message: "Adding files to staging",
						created_at: createTimestamp(1, 44), // 15:01:44
						uri: "",
						state: "complete" as const,
					},
					{
						// (15:01:32)
						...MockWorkspaceAppStatus,
						id: "status-2",
						icon: "/emojis/1f33f.png", // 🌿
						message: "Creating a new branch for PR",
						created_at: createTimestamp(1, 32), // 15:01:32
						uri: "",
						state: "complete" as const,
					},
					{
						// (15:01:00) - Oldest
						...MockWorkspaceAppStatus,
						id: "status-1",
						icon: "/emojis/1f680.png", // 🚀
						message: "Starting to create a PR",
						created_at: createTimestamp(1, 0), // 15:01:00
						uri: "",
						state: "complete" as const,
					},
				].sort(
					(a, b) =>
						new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
				), // Ensure sorted correctly for component input if needed
			},
		],
		// Pass the reference date to the component for Storybook rendering
		referenceDate: storyReferenceDate,
	},
};

// Add a story with a "Working" status as the latest
export const WorkingState: Story = {
	args: {
		workspace: MockWorkspace,
		agents: [MockWorkspaceAgent],
		apps: [
			{
				...MockWorkspaceApp,
				statuses: [
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
					{
						// Previous latest (15:04:38)
						...MockWorkspaceAppStatus,
						id: "status-7",
						icon: "/emojis/1f4dd.png", // 📝
						message: "Creating PR with gh CLI",
						created_at: createTimestamp(4, 38), // 15:04:38
						uri: "https://github.com/coder/coder/pull/5678",
						state: "complete" as const,
					},
					{
						// (15:03:56)
						...MockWorkspaceAppStatus,
						id: "status-6",
						icon: "/emojis/1f680.png", // 🚀
						message: "Pushing branch to remote",
						created_at: createTimestamp(3, 56), // 15:03:56
						uri: "",
						state: "complete" as const,
					},
					// ... include other older statuses if desired ...
					{
						// (15:01:00) - Oldest
						...MockWorkspaceAppStatus,
						id: "status-1",
						icon: "/emojis/1f680.png", // 🚀
						message: "Starting to create a PR",
						created_at: createTimestamp(1, 0), // 15:01:00
						uri: "",
						state: "complete" as const,
					},
				].sort(
					(a, b) =>
						new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
				),
			},
		],
		referenceDate: storyReferenceDate, // Use the same reference date
	},
};
