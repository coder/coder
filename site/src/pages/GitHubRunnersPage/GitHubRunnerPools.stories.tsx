import type { Meta, StoryObj } from "@storybook/react-vite";
import { focusManager } from "react-query";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	githubRunnerPoolsKey,
	githubRunnersKey,
} from "#/api/queries/githubRunners";
import { templatesByOrganizationKey } from "#/api/queries/templates";
import type { GitHubRunner, GitHubRunnerPool } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockGitHubRunner,
	MockGitHubRunnerPool,
	MockNoOrganizationPermissions,
	MockOrganizationPermissions,
	MockTemplate,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { withAuthProvider } from "#/testHelpers/storybook";
import { GitHubRunnerPools } from "./GitHubRunnerPools";

const MockPoolStates: GitHubRunnerPool[] = [
	{
		...MockGitHubRunnerPool,
		id: "paused",
		state: "paused",
		name: "paused-pool",
		assigned_jobs: 0,
	},
	{
		...MockGitHubRunnerPool,
		id: "draining",
		state: "draining",
		name: "draining-pool",
	},
	{
		...MockGitHubRunnerPool,
		id: "deleted",
		state: "deleted",
		name: "deleted-pool",
		assigned_jobs: 0,
	},
	{
		...MockGitHubRunnerPool,
		id: "error",
		name: "unhealthy-pool",
		last_error:
			"GitHub rejected the runner token. Check the token's permissions.",
	},
];
const MockCompletedRunner: GitHubRunner = {
	...MockGitHubRunner,
	id: "completed",
	state: "deleted",
	job_result: "succeeded",
};

const MockFailedRunner: GitHubRunner = {
	...MockGitHubRunner,
	id: "failed",
	state: "failed",
	workspace_id: "00000000-0000-0000-0000-000000000000",
	last_error: "Workspace provisioning failed. Check the template's build logs.",
};
const MockPausedPool: GitHubRunnerPool = {
	...MockGitHubRunnerPool,
	state: "paused",
};

const meta = {
	title: "pages/GitHubRunnersPage/Pools",
	component: GitHubRunnerPools,
	decorators: [withAuthProvider],
	args: {
		organization: MockDefaultOrganization,
		permissions: MockOrganizationPermissions,
	},
	parameters: { user: MockUserOwner },
	beforeEach: () => {
		// Keep polling from replacing the seeded visual states during capture.
		focusManager.setFocused(false);
		return () => focusManager.setFocused(undefined);
	},
} satisfies Meta<typeof GitHubRunnerPools>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Pools: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
};
export const Empty: Story = {
	parameters: {
		queries: [
			{ key: githubRunnerPoolsKey(MockDefaultOrganization.id), data: [] },
		],
	},
};
export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getGitHubRunnerPools").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API, "getGitHubRunnerPools").mockRejectedValue(
			new Error(
				"Unable to load GitHub runner pools. Check your connection and retry.",
			),
		);
	},
};
export const PoolStates: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: MockPoolStates,
			},
		],
	},
};
export const ReadOnly: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	args: {
		permissions: {
			...MockNoOrganizationPermissions,
			viewGitHubRunnerPools: true,
		},
	},
};
export const RunnerDetails: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
			{
				key: githubRunnersKey(
					MockDefaultOrganization.id,
					MockGitHubRunnerPool.id,
				),
				data: [MockGitHubRunner, MockCompletedRunner, MockFailedRunner],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: MockGitHubRunnerPool.name,
			}),
		);
	},
};
export const RunnersEmpty: Story = {
	play: RunnerDetails.play,
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
			{
				key: githubRunnersKey(
					MockDefaultOrganization.id,
					MockGitHubRunnerPool.id,
				),
				data: [],
			},
		],
	},
};
export const RunnersLoading: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getGitHubRunners").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: RunnerDetails.play,
};
export const RunnersError: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getGitHubRunners").mockRejectedValue(
			new Error("Runner allocations are unavailable. Retry the request."),
		);
	},
	play: RunnerDetails.play,
};
export const CreatePool: Story = {
	parameters: {
		queries: [
			{ key: githubRunnerPoolsKey(MockDefaultOrganization.id), data: [] },
			{
				key: templatesByOrganizationKey(MockDefaultOrganization.id),
				data: [MockTemplate],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Create pool" }),
		);
	},
};
export const EditCapacity: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Manage ${MockGitHubRunnerPool.name}`,
			}),
		);
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Edit capacity" }),
		);
	},
};
export const DeletePool: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Manage ${MockGitHubRunnerPool.name}`,
			}),
		);
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Drain and delete" }),
		);
	},
};
export const Mobile: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
		pixel: { matrix: pixelWithPhone },
	},
	globals: { viewport: { value: "iphone12" } },
};

export const ActiveActions: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Manage ${MockGitHubRunnerPool.name}`,
			}),
		);
	},
};
export const PausedActions: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockPausedPool],
			},
		],
	},
	play: ActiveActions.play,
};
export const ReadOnlyActions: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	args: {
		permissions: {
			...MockNoOrganizationPermissions,
			viewGitHubRunnerPools: true,
		},
	},
	play: ActiveActions.play,
};
export const PauseError: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateGitHubRunnerPool").mockRejectedValue(
			new Error("Could not pause this pool. Check your permissions and retry."),
		);
	},
	play: async (context) => {
		await ActiveActions.play?.(context);
		await userEvent.click(screen.getByRole("menuitem", { name: "Pause" }));
	},
};
export const ResumeError: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockPausedPool],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateGitHubRunnerPool").mockRejectedValue(
			new Error(
				"Could not resume this pool. Check your permissions and retry.",
			),
		);
	},
	play: async (context) => {
		await ActiveActions.play?.(context);
		await userEvent.click(screen.getByRole("menuitem", { name: "Resume" }));
	},
};
export const CapacityError: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateGitHubRunnerPool").mockRejectedValue(
			new Error("Could not update capacity. Check your permissions and retry."),
		);
	},
	play: async (context) => {
		await EditCapacity.play?.(context);
		await userEvent.click(
			screen.getByRole("button", { name: "Save capacity" }),
		);
	},
};
export const DeleteError: Story = {
	parameters: {
		queries: [
			{
				key: githubRunnerPoolsKey(MockDefaultOrganization.id),
				data: [MockGitHubRunnerPool],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "deleteGitHubRunnerPool").mockRejectedValue(
			new Error(
				"Could not delete this pool. Check your permissions and retry.",
			),
		);
	},
	play: async (context) => {
		await DeletePool.play?.(context);
		await userEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", {
				name: "Drain and delete",
			}),
		);
	},
};
