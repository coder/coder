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
		onModelChange: fn(),
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
		const input = canvas.getByPlaceholderText("Type a message...");
		const sendButton = canvas.getByRole("button", { name: "Send" });

		expect(sendButton).toBeDisabled();
		await userEvent.type(input, "Write tests");
		expect(sendButton).toBeEnabled();
	},
};

export const SendsAndClearsInput: Story = {
	args: {
		onSend: fn().mockResolvedValue(undefined),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByPlaceholderText("Type a message...");

		await userEvent.type(input, "Run focused tests");
		await userEvent.click(canvas.getByRole("button", { name: "Send" }));

		await waitFor(() => {
			expect(args.onSend).toHaveBeenCalledWith("Run focused tests", undefined);
		});
		expect(input).toHaveValue("");
	},
};

export const DisabledInput: Story = {
	args: {
		isDisabled: true,
		initialValue: "Should not send",
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
		initialValue: "Model required",
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
		initialValue: "Sending...",
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
