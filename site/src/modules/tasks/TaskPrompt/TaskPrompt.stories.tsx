import {
	MockAIPromptPresets,
	MockPresets,
	MockTask,
	MockTasks,
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	mockApiError,
} from "testHelpers/entities";
import { withAuthProvider, withGlobalSnackbar } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type { Task } from "api/typesGenerated";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import type TasksPage from "../../../pages/TasksPage/TasksPage";
import { TaskPrompt } from "./TaskPrompt";

const MockNewTaskData: Task = {
	...MockTask,
	current_state: {
		...MockTask.current_state,
		message: "Task created successfully!",
	},
};

const meta: Meta<typeof TasksPage> = {
	title: "modules/tasks/TaskPrompt",
	component: TaskPrompt,
	decorators: [withAuthProvider],
	parameters: {
		user: MockUserOwner,
		permissions: {
			updateTemplates: true,
		},
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		spyOn(API, "getTemplateVersions").mockResolvedValue([
			{
				...MockTemplateVersion,
				name: "v1.0.0",
			},
		]);
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue(null);
	},
	args: {
		templates: [MockTemplate],
	},
};

export default meta;
type Story = StoryObj<typeof TasksPage>;

export const LoadingTemplates: Story = {
	args: {
		templates: undefined,
	},
};

export const EmptyTemplates: Story = {
	args: {
		templates: [],
	},
};

export const WithPresets: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue(MockPresets);
	},
};

export const ReadOnlyPresetPrompt: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue(
			MockAIPromptPresets,
		);
	},
};

export const SubmitEnabledWhenPromptNotEmpty: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const prompt = await canvas.findByLabelText(/prompt/i);
		await userEvent.type(prompt, MockNewTaskData.initial_prompt);

		const submitButton = canvas.getByRole("button", { name: /run task/i });
		expect(submitButton).toBeEnabled();
	},
};

export const SubmitDisabledWhenPromptEmpty: Story = {
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("No prompt", async () => {
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			expect(submitButton).toBeDisabled();
		});

		await step("Whitespace prompt", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, "   ");

			const submitButton = canvas.getByRole("button", { name: /run task/i });
			expect(submitButton).toBeDisabled();
		});
	},
};

export const Submitting: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API.tasks, "createTask").mockImplementation(
			() =>
				// Never resolve to keep the component in the submitting state for visual testing.
				new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const prompt = await canvas.findByLabelText(/prompt/i);
		await userEvent.type(
			prompt,
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit.{enter}{enter}Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
		);

		const submitButton = canvas.getByRole("button", { name: /run task/i });
		await waitFor(() => expect(submitButton).toBeEnabled());
		await userEvent.click(submitButton);
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};

export const OnSuccess: Story = {
	decorators: [withGlobalSnackbar],
	parameters: {
		permissions: {
			updateTemplates: false,
		},
	},
	beforeEach: () => {
		const activeVersionId = `${MockTemplate.active_version_id}-latest`;
		spyOn(API, "getTemplate").mockResolvedValue({
			...MockTemplate,
			active_version_id: activeVersionId,
		});
		spyOn(API.tasks, "createTask").mockResolvedValue(MockTask);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Run task", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, MockNewTaskData.initial_prompt);
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			await waitFor(() => expect(submitButton).toBeEnabled());
			await userEvent.click(submitButton);
		});

		await step("Uses latest template version", () => {
			expect(API.tasks.createTask).toHaveBeenCalledWith(
				MockUserOwner.id,
				{
					input: MockNewTaskData.initial_prompt,
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

		await step("Clears prompt", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			expect(prompt).toHaveValue("");
		});
	},
};

export const ChangeTemplate: Story = {
	decorators: [withGlobalSnackbar],
	args: {
		templates: [
			{
				...MockTemplate,
				id: "claude-code",
				name: "claude-code",
				display_name: "Claude Code",
				active_version_id: "claude-code-version",
			},
			{
				...MockTemplate,
				id: "codex",
				name: "codex",
				display_name: "Codex",
				active_version_id: "codex-version",
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersions").mockImplementation((templateId) => {
			if (templateId === "claude-code") {
				return Promise.resolve([
					{
						...MockTemplateVersion,
						id: "claude-code-version",
						name: "claude-code-version",
					},
				]);
			}
			if (templateId === "codex") {
				return Promise.resolve([
					{
						...MockTemplateVersion,
						id: "codex-version",
						name: "codex-version",
					},
				]);
			}
			return Promise.resolve([]);
		});
		spyOn(API.tasks, "createTask").mockResolvedValue(MockTask);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await step("Change template", async () => {
			const templateSelect = await canvas.findByLabelText(/select template/i);
			await userEvent.click(templateSelect);
			const templateOption = await body.findByRole("option", {
				name: /codex/i,
			});
			await userEvent.click(templateOption);
		});

		await step("Default version is selected", async () => {
			const versionSelect = await canvas.findByLabelText(/version/i);
			expect(versionSelect).toHaveTextContent("codex-version");
		});
	},
};

export const SelectTemplateVersion: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API, "getTemplateVersions").mockResolvedValue([
			{
				...MockTemplateVersion,
				id: "test-template-version-2",
				name: "v2.0.0",
			},
			{
				...MockTemplateVersion,
				name: "v1.0.0",
			},
		]);
		spyOn(API.tasks, "createTask").mockResolvedValue(MockTask);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Fill prompt", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, MockNewTaskData.initial_prompt);
		});

		await step("Select version", async () => {
			const body = within(canvasElement.ownerDocument.body);
			const versionSelect = await canvas.findByLabelText(/template version/i);
			await userEvent.click(versionSelect);
			const versionOption = await body.findByRole("option", {
				name: /v2.0.0/i,
			});
			await userEvent.click(versionOption);
		});

		await step("Submit form", async () => {
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			await waitFor(() => expect(submitButton).toBeEnabled());
			await userEvent.click(submitButton);
		});

		await step("Uses selected version", () => {
			expect(API.tasks.createTask).toHaveBeenCalledWith(
				MockUserOwner.id,
				{
					input: MockNewTaskData.initial_prompt,
					template_version_id: "test-template-version-2",
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

export const OnError: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		spyOn(API.tasks, "getTasks").mockResolvedValue(MockTasks);
		spyOn(API.tasks, "createTask").mockRejectedValue(
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

export const AuthenticatedExternalAuth: Story = {
	beforeEach: () => {
		spyOn(API.tasks, "getTasks")
			.mockResolvedValueOnce(MockTasks)
			.mockResolvedValue([MockNewTaskData, ...MockTasks]);
		spyOn(API.tasks, "createTask").mockResolvedValue(MockTask);
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
		spyOn(API.tasks, "getTasks")
			.mockResolvedValueOnce(MockTasks)
			.mockResolvedValue([MockNewTaskData, ...MockTasks]);
		spyOn(API.tasks, "createTask").mockResolvedValue(MockTask);
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
			MockTemplateVersionExternalAuthGithub,
		]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Submit is disabled", async () => {
			const prompt = await canvas.findByLabelText(/prompt/i);
			await userEvent.type(prompt, MockNewTaskData.initial_prompt);
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
		spyOn(API.tasks, "getTasks")
			.mockResolvedValueOnce(MockTasks)
			.mockResolvedValue([MockNewTaskData, ...MockTasks]);
		spyOn(API.tasks, "createTask").mockResolvedValue(MockTask);
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
			await userEvent.type(prompt, MockNewTaskData.initial_prompt);
			const submitButton = canvas.getByRole("button", { name: /run task/i });
			expect(submitButton).toBeDisabled();
		});

		await step("Renders error", async () => {
			await canvas.findByText(/failed to load external auth/i);
		});
	},
};

const tmplWithExternalAuth = {
	...MockTemplateVersion,
	id: "2",
	name: "With external",
};

export const CheckExternalAuthOnChangingVersions: Story = {
	args: {
		templates: [
			{
				...MockTemplate,
				active_version_id: tmplWithExternalAuth.id,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersions").mockResolvedValue([
			{
				...MockTemplateVersion,
				id: "1",
				name: "No external",
			},
			tmplWithExternalAuth,
		]);
		spyOn(API, "getTemplateVersionExternalAuth").mockImplementation(
			(versionId: string) => {
				return Promise.resolve(
					versionId === tmplWithExternalAuth.id
						? [MockTemplateVersionExternalAuthGithub]
						: [],
				);
			},
		);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Renders external authentication", async () => {
			await canvas.findByRole("button", { name: /connect to github/i });
		});

		await step("Change into version without external auth", async () => {
			const body = within(canvasElement.ownerDocument.body);
			const versionSelect = await canvas.findByLabelText(/template version/i);
			await userEvent.click(versionSelect);
			const versionOption = await body.findByRole("option", {
				name: /no external/i,
			});
			await userEvent.click(versionOption);
		});

		await step("Don't render external authentication", async () => {
			expect(
				canvas.queryByRole("button", { name: /connect to github/i }),
			).not.toBeInTheDocument();
		});
	},
};

export const CheckPresetsWhenChangingTemplate: Story = {
	args: {
		templates: [
			{
				...MockTemplate,
				id: "claude-code",
				name: "claude-code",
				display_name: "Claude Code",
				active_version_id: "claude-code-version",
			},
			{
				...MockTemplate,
				id: "codex",
				name: "codex",
				display_name: "Codex",
				active_version_id: "codex-version",
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersionPresets").mockImplementation((versionId) => {
			if (versionId === "claude-code-version") {
				return Promise.resolve([
					{
						...MockPresets[0],
						ID: "claude-code-preset-1",
						Name: "Claude Code Dev",
					},
				]);
			}
			if (versionId === "codex-version") {
				return Promise.resolve([
					{
						...MockPresets[0],
						ID: "codex-preset-1",
						Name: "Codex Dev",
					},
				]);
			}
			return Promise.resolve([]);
		});
		spyOn(API, "getTemplateVersions").mockImplementation((templateId) => {
			if (templateId === "claude-code") {
				return Promise.resolve([
					{
						...MockTemplateVersion,
						id: "claude-code-version",
						name: "claude-code-version",
					},
				]);
			}
			if (templateId === "codex") {
				return Promise.resolve([
					{
						...MockTemplateVersion,
						id: "codex-version",
						name: "codex-version",
					},
				]);
			}
			return Promise.resolve([]);
		});
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await step("Presets are initially present", async () => {
			const presetSelect = await canvas.findByLabelText(/preset/i);
			await userEvent.click(presetSelect);

			const options = await body.findAllByRole("option");
			expect(options).toHaveLength(1);
			expect(options[0]).toContainHTML("Claude Code Dev");

			await userEvent.click(options[0]);
		});

		await step("Switch template", async () => {
			const templateSelect = await canvas.findByLabelText(/select template/i);
			await userEvent.click(templateSelect);

			const codexTemplateOption = await body.findByRole("option", {
				name: /codex/i,
			});
			await userEvent.click(codexTemplateOption);
		});

		await step("Presets are present in new template", async () => {
			const presetSelect = await canvas.findByLabelText(/preset/i);
			await userEvent.click(presetSelect);

			const options = await body.findAllByRole("option");
			expect(options).toHaveLength(1);
			expect(options[0]).toContainHTML("Codex Dev");

			await userEvent.click(options[0]);
		});

		await step("Switch template back", async () => {
			const templateSelect = await canvas.findByLabelText(/select template/i);
			await userEvent.click(templateSelect);

			const codexTemplateOption = await body.findByRole("option", {
				name: /claude code/i,
			});
			await userEvent.click(codexTemplateOption);
		});

		await step("Presets are present in original template", async () => {
			const presetSelect = await canvas.findByLabelText(/preset/i);
			await userEvent.click(presetSelect);

			const options = await body.findAllByRole("option");
			expect(options).toHaveLength(1);
			expect(options[0]).toContainHTML("Claude Code Dev");
		});
	},
};
