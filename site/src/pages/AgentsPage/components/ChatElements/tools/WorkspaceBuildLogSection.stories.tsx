import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceBuildLogSection } from "./WorkspaceBuildLogSection";

const TEST_BUILD_ID = "test-build-id-000";
const TEST_WORKSPACE_ID = "test-workspace-id-000";

const sampleLogs: ProvisionerJobLog[] = [
	{
		id: 1,
		created_at: "2024-01-01T00:00:00Z",
		log_source: "provisioner",
		log_level: "info",
		stage: "Starting workspace",
		output: "Initializing Terraform...",
	},
	{
		id: 2,
		created_at: "2024-01-01T00:00:01Z",
		log_source: "provisioner",
		log_level: "info",
		stage: "Starting workspace",
		output: "Terraform has been successfully initialized!",
	},
	{
		id: 3,
		created_at: "2024-01-01T00:00:02Z",
		log_source: "provisioner",
		log_level: "info",
		stage: "Starting workspace",
		output: "Apply complete! Resources: 2 added, 0 changed, 0 destroyed.",
	},
];

const meta: Meta<typeof WorkspaceBuildLogSection> = {
	title: "pages/AgentsPage/ChatElements/tools/WorkspaceBuildLogSection",
	component: WorkspaceBuildLogSection,
	decorators: [
		(Story) => (
			<ChatWorkspaceContext value={{ workspaceId: TEST_WORKSPACE_ID }}>
				<Story />
			</ChatWorkspaceContext>
		),
	],
};

export default meta;
type Story = StoryObj<typeof WorkspaceBuildLogSection>;

/** Build ID is present but the REST fetch has not resolved yet. */
export const Loading: Story = {
	args: {
		status: "completed",
		buildId: TEST_BUILD_ID,
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceBuildLogs").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};

/** Completed build with logs fetched from the REST endpoint. */
export const CompletedWithLogs: Story = {
	args: {
		status: "completed",
		buildId: TEST_BUILD_ID,
	},
	parameters: {
		queries: [
			{
				key: ["workspaceBuilds", TEST_BUILD_ID, "logs"],
				data: sampleLogs,
			},
		],
	},
};

/** REST fetch for build logs returned a server error. */
export const FetchError: Story = {
	args: {
		status: "completed",
		buildId: TEST_BUILD_ID,
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceBuildLogs").mockRejectedValue(
			new Error("Internal Server Error"),
		);
	},
};

/**
 * Build completed with zero log output. The REST query succeeds but
 * returns an empty array, so the component shows "No build logs
 * available." instead of a perpetual spinner.
 */
export const CompletedEmptyLogs: Story = {
	args: {
		status: "completed",
		buildId: TEST_BUILD_ID,
	},
	parameters: {
		queries: [
			{
				key: ["workspaceBuilds", TEST_BUILD_ID, "logs"],
				data: [],
			},
		],
	},
};
