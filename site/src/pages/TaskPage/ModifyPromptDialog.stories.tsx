import {
	MockTask,
	MockTaskWorkspace,
	mockApiError,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { workspaceBuildParametersKey } from "api/queries/workspaceBuilds";
import {
	AITaskPromptParameterName,
	type Workspace,
	type WorkspaceBuildParameter,
} from "api/typesGenerated";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { ModifyPromptDialog } from "./ModifyPromptDialog";

const mockTaskWorkspaceStarting: Workspace = {
	...MockTaskWorkspace,
	latest_build: {
		...MockTaskWorkspace.latest_build,
		status: "starting",
	},
};

// Mock build parameters for the workspace
const mockBuildParameters: WorkspaceBuildParameter[] = [
	{
		name: AITaskPromptParameterName,
		value: MockTask.initial_prompt,
	},
	{
		name: "region",
		value: "us-east-1",
	},
];

const meta: Meta<typeof ModifyPromptDialog> = {
	title: "pages/TaskPage/ModifyPromptDialog",
	component: ModifyPromptDialog,
	args: {
		task: MockTask,
		workspace: mockTaskWorkspaceStarting,
		open: true,
		onOpenChange: () => {},
	},
	parameters: {
		queries: [
			{
				key: workspaceBuildParametersKey(MockTaskWorkspace.latest_build.id),
				data: mockBuildParameters,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ModifyPromptDialog>;

export const WithModifiedPrompt: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const promptTextarea = body.getByLabelText("Prompt");

		// Given: The user modifies the prompt
		await userEvent.clear(promptTextarea);
		await userEvent.type(promptTextarea, "Build a web server in Go");

		// Then: We expect the submit button to not be disabled
		const submitButton = body.getByRole("button", {
			name: /update and restart build/i,
		});
		expect(submitButton).not.toBeDisabled();
	},
};

export const EmptyPrompt: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const promptTextarea = body.getByLabelText("Prompt");

		// Given: The prompt is empty
		await userEvent.clear(promptTextarea);

		// Then: We expect the submit button to be disabled
		const submitButton = body.getByRole("button", {
			name: /update and restart build/i,
		});
		expect(submitButton).toBeDisabled();
	},
};

export const UnchangedPrompt: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Given: The prompt is unchanged

		// Then: We expect the submit button to be disabled
		const submitButton = body.getByRole("button", {
			name: /update and restart build/i,
		});
		expect(submitButton).toBeDisabled();
	},
};

export const Submitting: Story = {
	beforeEach: async () => {
		// Mock all API calls that happen before updateTaskPrompt
		spyOn(API, "cancelWorkspaceBuild").mockResolvedValue({
			message: "Workspace build canceled",
		});
		spyOn(API, "getWorkspaceBuildByNumber").mockResolvedValue({
			...MockTaskWorkspace.latest_build,
			status: "canceled",
		});
		spyOn(API, "waitForBuild").mockResolvedValue(undefined);
		spyOn(API, "stopWorkspace").mockResolvedValue(
			MockTaskWorkspace.latest_build,
		);
		// Mock updateTaskPrompt to never resolve (keeps it in pending state)
		spyOn(API, "updateTaskInput").mockImplementation(() => {
			return new Promise(() => {});
		});
		spyOn(API, "startWorkspace").mockResolvedValue(
			MockTaskWorkspace.latest_build,
		);
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("Modify and submit the form", async () => {
			const promptTextarea = body.getByLabelText("Prompt");
			await userEvent.clear(promptTextarea);
			await userEvent.type(promptTextarea, "Create a REST API");

			const submitButton = body.getByRole("button", {
				name: /update and restart build/i,
			});
			await userEvent.click(submitButton);
		});

		await step("Shows loading state with spinner", async () => {
			const spinner = await body.findByTitle("Loading spinner");
			expect(spinner).toBeInTheDocument();

			const submitButton = body.getByRole("button", {
				name: /update and restart build/i,
			});
			expect(submitButton).toBeDisabled();
		});
	},
};

export const Success: Story = {
	beforeEach: async () => {
		spyOn(API, "updateTaskInput").mockResolvedValue();
		spyOn(API, "cancelWorkspaceBuild").mockResolvedValue({
			message: "Workspace build canceled",
		});
		spyOn(API, "getWorkspaceBuildByNumber").mockResolvedValue({
			...MockTaskWorkspace.latest_build,
			status: "canceled",
		});
		spyOn(API, "waitForBuild").mockResolvedValue(undefined);
		spyOn(API, "stopWorkspace").mockResolvedValue(
			MockTaskWorkspace.latest_build,
		);
		spyOn(API, "startWorkspace").mockResolvedValue(
			MockTaskWorkspace.latest_build,
		);
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("Modify and submit the form", async () => {
			const promptTextarea = body.getByLabelText("Prompt");
			await userEvent.clear(promptTextarea);
			await userEvent.type(promptTextarea, "Create a REST API in Python");

			const submitButton = body.getByRole("button", {
				name: /update and restart build/i,
			});
			await userEvent.click(submitButton);
		});

		await step("API calls are made", async () => {
			await waitFor(() => {
				expect(API.cancelWorkspaceBuild).toHaveBeenCalledWith(
					mockTaskWorkspaceStarting.latest_build.id,
				);
				expect(API.stopWorkspace).toHaveBeenCalledWith(
					mockTaskWorkspaceStarting.id,
				);
				expect(API.updateTaskInput).toHaveBeenCalledWith(
					MockTask.owner_name,
					MockTask.id,
					"Create a REST API in Python",
				);
				expect(API.startWorkspace).toHaveBeenCalledWith(
					mockTaskWorkspaceStarting.id,
					MockTask.template_version_id,
					undefined,
					[
						{
							name: AITaskPromptParameterName,
							value: "Create a REST API in Python",
						},
						{
							name: "region",
							value: "us-east-1",
						},
					],
				);
			});
		});
	},
};

export const Failure: Story = {
	beforeEach: async () => {
		// Mock all API calls that happen before updateTaskPrompt
		spyOn(API, "cancelWorkspaceBuild").mockResolvedValue({
			message: "Workspace build canceled",
		});
		spyOn(API, "getWorkspaceBuildByNumber").mockResolvedValue({
			...MockTaskWorkspace.latest_build,
			status: "canceled",
		});
		spyOn(API, "waitForBuild").mockResolvedValue(undefined);
		spyOn(API, "stopWorkspace").mockResolvedValue(
			MockTaskWorkspace.latest_build,
		);
		// Mock updateTaskPrompt to reject with an error
		spyOn(API, "updateTaskInput").mockRejectedValue(
			mockApiError({
				message: "Failed to update task prompt",
				detail: "Build is not in a valid state for modification",
			}),
		);
		// Don't need to mock startWorkspace since it won't be reached after the error
	},
	play: async ({ canvasElement, step }) => {
		const body = within(canvasElement.ownerDocument.body);

		await step("Modify and submit the form", async () => {
			const promptTextarea = body.getByLabelText("Prompt");
			await userEvent.clear(promptTextarea);
			await userEvent.type(promptTextarea, "Create a REST API");

			const submitButton = body.getByRole("button", {
				name: /update and restart build/i,
			});
			await userEvent.click(submitButton);
		});

		await step("Shows error message", async () => {
			await body.findByText(/Failed to update task prompt/i);
		});
	},
};

export const RunningBuild: Story = {
	args: {
		workspace: {
			...MockTaskWorkspace,
			latest_build: {
				...MockTaskWorkspace.latest_build,
				status: "running",
			},
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Verify error message is displayed
		expect(
			body.getByText(/Cannot modify the prompt of a running task/i),
		).toBeInTheDocument();

		// Verify submit button is disabled
		const submitButton = body.getByRole("button", {
			name: /update and restart build/i,
		});
		expect(submitButton).toBeDisabled();
	},
};
