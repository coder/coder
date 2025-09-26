import {
	MockNewTaskData,
	MockPresets,
	MockTask,
	MockTemplate,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	mockApiError,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withGlobalSnackbar,
	withProxyProvider,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { AI_PROMPT_PARAMETER_NAME } from "modules/tasks/tasks";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import TasksPage from "./TasksPage";

const meta: Meta<typeof TasksPage> = {
	title: "pages/TasksPage",
	component: TasksPage,
	decorators: [withAuthProvider, withProxyProvider()],
	parameters: {
		user: MockUserOwner,
		permissions: {
			viewDeploymentConfig: true,
		},
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue(null);
		spyOn(API, "getUsers").mockResolvedValue({
			users: MockUsers,
			count: MockUsers.length,
		});
		spyOn(API, "getTemplates").mockResolvedValue([
			MockTemplate,
			{
				...MockTemplate,
				id: "test-template-2",
				name: "template 2",
				display_name: "Template 2",
			},
		]);
	},
};

export default meta;
type Story = StoryObj<typeof TasksPage>;

export const LoadingTemplates: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockImplementation(
			() => new Promise(() => 1000 * 60 * 60),
		);
	},
};

export const LoadingTemplatesError: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockRejectedValue(
			mockApiError({
				message: "Failed to load AI templates",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const NoTemplates: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([]);
	},
};

export const NoPreset: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
	},
};

export const WithPreset: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue(MockPresets);
	},
};

export const PreDefinedPrompt: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
			{
				...MockPresets[0],
				Parameters: [
					{ Name: AI_PROMPT_PARAMETER_NAME, Value: "Write a poem about AI" },
				],
			},
		]);
	},
};

export const CreateTaskSuccessfully: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		const activeVersionId = `${MockTemplate.active_version_id}-latest`;
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplate").mockResolvedValue({
			...MockTemplate,
			active_version_id: activeVersionId,
		});
		spyOn(API.experimental, "createTask").mockResolvedValue(MockTask);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Run task", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, MockNewTaskData.prompt);
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			await waitFor(() => expect(submitButton).toBeEnabled());
			await userEvent.click(submitButton);
		});

		await step("Uses latest template version", () => {
			expect(API.experimental.createTask).toHaveBeenCalledWith(
				MockUserOwner.id,
				{
					input: MockNewTaskData.prompt,
					template_version_id: `${MockTemplate.active_version_id}-latest`,
					template_version_preset_id: undefined,
				},
			);
		});

		await step("Displays success message", async () => {
			const body = within(canvasElement.ownerDocument.body);
			const successMessage = await body.findByText(/task created/i);
			expect(successMessage).toBeInTheDocument();
		});
	},
};

export const CreateTaskError: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		spyOn(API.experimental, "createTask").mockRejectedValue(
			mockApiError({
				message: "Failed to create task",
				detail: "You don't have permission to create tasks.",
			}),
		);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Run task", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, "Create a new task");
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			await waitFor(() => expect(submitButton).toBeEnabled());
			await userEvent.click(submitButton);
		});

		await step("Verify error", async () => {
			await canvas.findByText(/failed to create task/i);
		});
	},
};

export const WithAuthenticatedExternalAuth: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
			MockTemplateVersionExternalAuthGithubAuthenticated,
		]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Does not render external auth", async () => {
			expect(
				canvas.queryByText(/external authentication/),
			).not.toBeInTheDocument();
		});
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};

export const MissingExternalAuth: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
			MockTemplateVersionExternalAuthGithub,
		]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Submit is disabled", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, MockNewTaskData.prompt);
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			expect(submitButton).toBeDisabled();
		});

		await step("Renders external authentication", async () => {
			await canvas.findByRole("button", { name: /connect to github/i });
		});
	},
};

export const ExternalAuthError: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
		spyOn(API, "getTemplateVersionExternalAuth").mockRejectedValue(
			mockApiError({
				message: "Failed to load external auth",
			}),
		);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Submit is disabled", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, MockNewTaskData.prompt);
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			expect(submitButton).toBeDisabled();
		});

		await step("Renders error", async () => {
			await canvas.findByText(/failed to load external auth/i);
		});
	},
};
