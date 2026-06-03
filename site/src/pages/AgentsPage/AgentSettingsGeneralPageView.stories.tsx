import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type { AgentChatSendShortcut } from "#/api/typesGenerated";
import {
	AgentSettingsGeneralPageView,
	type AgentSettingsGeneralPageViewProps,
} from "./AgentSettingsGeneralPageView";

const preferencesData = {
	task_notification_alert_dismissed: false,
	thinking_display_mode: "auto" as const,
	shell_tool_display_mode: "auto" as const,
	code_diff_display_mode: "auto" as const,
	agent_chat_send_shortcut: "enter" as const,
};

const baseArgs: AgentSettingsGeneralPageViewProps = {
	userPromptData: {
		custom_prompt: "Prefer concise answers with clear next steps.",
	},
	onSaveUserPrompt: fn(),
	isSavingUserPrompt: false,
	isSaveUserPromptError: false,
	userDebugLoggingData: {
		debug_logging_enabled: false,
		user_toggle_allowed: false,
		forced_by_deployment: false,
	},
	onSaveUserDebugLogging: fn(),
	isSavingUserDebugLogging: false,
	isSaveUserDebugLoggingError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsGeneralPageView",
	component: AgentSettingsGeneralPageView,
	args: baseArgs,
	parameters: {
		queries: [{ key: ["me", "preferences"], data: preferencesData }],
	},
} satisfies Meta<typeof AgentSettingsGeneralPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsGeneralPageView>;

export const Default: Story = {};

export const InvisibleUnicodeWarningUserPrompt: Story = {
	args: {
		userPromptData: {
			custom_prompt: "My custom prompt\u200b\u200c\u200dhidden",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Personal instructions");
		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("2");
	},
};

export const InvisibleUnicodeWarningOnType: Story = {
	args: {
		userPromptData: {
			custom_prompt: "",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const textarea = await canvas.findByPlaceholderText(
			"Additional behavior, style, and tone preferences",
		);

		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();
		await userEvent.type(textarea, "hello\u200bworld");

		await waitFor(() => {
			expect(canvas.getByText(/invisible Unicode/)).toBeInTheDocument();
		});
	},
};

export const SavesUserPrompt: Story = {
	args: {
		userPromptData: {
			custom_prompt: "",
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const textarea = await canvas.findByPlaceholderText(
			"Additional behavior, style, and tone preferences",
		);

		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();
		await userEvent.type(
			textarea,
			"Prefer concise answers with clear next steps.",
		);

		const promptForm = textarea.closest("form");
		if (!(promptForm instanceof HTMLFormElement)) {
			throw new Error(
				"Expected personal instructions textarea to live inside a form.",
			);
		}
		const saveButton = within(promptForm).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveUserPrompt).toHaveBeenCalledWith(
				{ custom_prompt: "Prefer concise answers with clear next steps." },
				expect.anything(),
			);
		});
	},
};

export const RendersChatLayoutSection: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(await canvas.findByText("Chat layout")).toBeInTheDocument();
		expect(
			await canvas.findByRole("switch", { name: "Full-width chat" }),
		).toBeInTheDocument();
	},
};

export const TogglesSendShortcut: Story = {
	beforeEach: () => {
		let agentChatSendShortcut: AgentChatSendShortcut =
			preferencesData.agent_chat_send_shortcut;
		spyOn(API, "getUserPreferenceSettings").mockImplementation(async () => ({
			...preferencesData,
			agent_chat_send_shortcut: agentChatSendShortcut,
		}));
		spyOn(API, "updateUserPreferenceSettings").mockImplementation(
			async (req) => {
				agentChatSendShortcut =
					req.agent_chat_send_shortcut ?? agentChatSendShortcut;
				return {
					...preferencesData,
					agent_chat_send_shortcut: agentChatSendShortcut,
				};
			},
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Require Cmd/Ctrl+Enter to send messages",
		});

		expect(await canvas.findByText("Keyboard shortcuts")).toBeInTheDocument();
		expect(toggle).not.toBeChecked();

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(API.updateUserPreferenceSettings).toHaveBeenCalledWith({
				agent_chat_send_shortcut: "modifier_enter",
			});
			expect(toggle).toBeChecked();
		});
	},
};

export const RendersAgentDisplayModeSettings: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(await canvas.findByText("Thinking display")).toBeVisible();
		expect(await canvas.findByText("Shell output display")).toBeVisible();
		expect(await canvas.findByText("Code diff display")).toBeVisible();
	},
};

export const ShowsChatDebugLoggingToggle: Story = {
	args: {
		userDebugLoggingData: {
			debug_logging_enabled: false,
			user_toggle_allowed: true,
			forced_by_deployment: false,
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable personal chat debug logging",
		});

		expect(
			await canvas.findByText("Record debug logs for my chats"),
		).toBeInTheDocument();
		await userEvent.click(toggle);
		await waitFor(() => {
			expect(args.onSaveUserDebugLogging).toHaveBeenCalledWith({
				debug_logging_enabled: true,
			});
		});
	},
};

export const HidesChatDebugLoggingToggle: Story = {
	args: {
		userDebugLoggingData: {
			debug_logging_enabled: false,
			user_toggle_allowed: false,
			forced_by_deployment: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByText("Record debug logs for my chats")).toBeNull();
		expect(
			canvas.queryByRole("switch", {
				name: "Enable personal chat debug logging",
			}),
		).toBeNull();
	},
};
