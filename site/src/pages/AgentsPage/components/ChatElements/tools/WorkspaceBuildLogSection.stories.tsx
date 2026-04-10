import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, waitFor, within } from "storybook/test";
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
			<ChatWorkspaceContext.Provider value={TEST_WORKSPACE_ID}>
				<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
					<Story />
				</div>
			</ChatWorkspaceContext.Provider>
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Loading build logs\u2026")).toBeInTheDocument();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Starting workspace")).toBeInTheDocument();
		});
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText("Failed to load build logs."),
			).toBeInTheDocument();
		});
	},
};

/**
 * No logs arrive within the LOG_LOAD_TIMEOUT_MS window (30 s).
 * The component shows a soft warning instead of the spinner.
 * In Storybook's UI the timeout message appears after 30 seconds
 * of real time; the play function verifies the initial loading
 * state that precedes it.
 */
export const Timeout: Story = {
	args: {
		status: "completed",
		buildId: TEST_BUILD_ID,
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceBuildLogs").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The loading indicator is shown while waiting for the
		// timeout to fire.
		expect(canvas.getByText("Loading build logs\u2026")).toBeInTheDocument();
	},
};
