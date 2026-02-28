import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { AgentChatInput } from "./AgentChatInput";

const defaultModelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const meta: Meta<typeof AgentChatInput> = {
	title: "pages/AgentsPage/AgentChatInput",
	component: AgentChatInput,
	args: {
		onSend: fn(),
		onChange: fn(),
		onModelChange: fn(),
		value: "",
		isDisabled: false,
		isLoading: false,
		selectedModel: defaultModelOptions[0].id,
		modelOptions: [...defaultModelOptions],
		modelSelectorPlaceholder: "Select model",
		hasModelOptions: true,
		inputStatusText: null,
		modelCatalogStatusMessage: null,
	},
};

export default meta;
type Story = StoryObj<typeof AgentChatInput>;

export const Default: Story = {};

export const DisablesSendUntilInput: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });

		expect(sendButton).toBeDisabled();
	},
};

export const SendsAndClearsInput: Story = {
	args: {
		onSend: fn(),
		value: "Run focused tests",
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.click(canvas.getByRole("button", { name: "Send" }));

		await waitFor(() => {
			expect(args.onSend).toHaveBeenCalledWith("Run focused tests");
		});
	},
};

export const DisabledInput: Story = {
	args: {
		isDisabled: true,
		value: "Should not send",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const NoModelOptions: Story = {
	args: {
		isDisabled: false,
		hasModelOptions: false,
		value: "Model required",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const LoadingSpinner: Story = {
	args: {
		isDisabled: true,
		isLoading: true,
		value: "Sending...",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });
		expect(sendButton).toBeDisabled();
		// The Loader2Icon renders with the animate-spin class when
		// isLoading is true.
		expect(sendButton.querySelector(".animate-spin")).toBeTruthy();
	},
};

export const LoadingDisablesSend: Story = {
	args: {
		isDisabled: false,
		isLoading: true,
		value: "Another message",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });
		// The send button should be disabled while a previous send is
		// in-flight, even though the textarea has content.
		expect(sendButton).toBeDisabled();
	},
};
