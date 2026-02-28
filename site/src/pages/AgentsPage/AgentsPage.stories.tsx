import { MockWorkspace } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { AgentsEmptyState } from "./AgentsPage";

const modelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const behaviorStorageKey = "agents.system-prompt";

const meta: Meta<typeof AgentsEmptyState> = {
	title: "pages/AgentsPage/AgentsEmptyState",
	component: AgentsEmptyState,
	args: {
		onCreateChat: fn(),
		isCreating: false,
		createError: undefined,
		modelCatalog: null,
		modelOptions: [...modelOptions],
		isModelCatalogLoading: false,
		modelConfigs: [],
		isModelConfigsLoading: false,
		modelCatalogError: undefined,
		canSetSystemPrompt: true,
		canManageChatModelConfigs: false,
		isConfigureAgentsDialogOpen: false,
		onConfigureAgentsDialogOpenChange: fn(),
	},
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
	},
};

export default meta;
type Story = StoryObj<typeof AgentsEmptyState>;

export const Default: Story = {};

export const WithWorkspaces: Story = {
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [
				{
					...MockWorkspace,
					id: "ws-1",
					name: "my-project",
					owner_name: "johndoe",
					owner_id: "user-1",
				},
				{
					...MockWorkspace,
					id: "ws-2",
					name: "my-project",
					owner_name: "janedoe",
					owner_id: "user-2",
				},
				{
					...MockWorkspace,
					id: "ws-3",
					name: "backend-api",
					owner_name: "johndoe",
					owner_id: "user-1",
				},
			],
			count: 3,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			const trigger = canvas.getByText("Workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(canvas.getByText("Workspace").closest("button")!);
		// Wait for the portalled dropdown to appear so Chromatic captures it.
		await within(canvasElement.ownerDocument.body).findByRole("listbox");
	},
};

export const SavesBehaviorPromptAndRestores: Story = {
	args: {
		isConfigureAgentsDialogOpen: true,
	},
	play: async () => {
		const dialog = await screen.findByRole("dialog");
		const textarea = await within(dialog).findByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);

		await userEvent.type(textarea, "You are a focused coding assistant.");
		await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(localStorage.getItem(behaviorStorageKey)).toBe(
				"You are a focused coding assistant.",
			);
		});
	},
};
