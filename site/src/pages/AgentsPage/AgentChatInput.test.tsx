import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { renderComponent } from "testHelpers/renderHelpers";
import { AgentChatInput } from "./AgentChatInput";

type AgentChatInputProps = ComponentProps<typeof AgentChatInput>;

const defaultModelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const buildProps = (
	overrides: Partial<AgentChatInputProps> = {},
): AgentChatInputProps => ({
	onSend: vi.fn().mockResolvedValue(undefined),
	isDisabled: false,
	isLoading: false,
	selectedModel: defaultModelOptions[0].id,
	onModelChange: vi.fn(),
	modelOptions: defaultModelOptions,
	modelSelectorPlaceholder: "Select model",
	hasModelOptions: true,
	inputStatusText: null,
	modelCatalogStatusMessage: null,
	...overrides,
});

describe(AgentChatInput.name, () => {
	it("disables send until there is input text", async () => {
		renderComponent(<AgentChatInput {...buildProps()} />);

		const input = screen.getByPlaceholderText("Type a message...");
		const sendButton = screen.getByRole("button", { name: "Send" });

		expect(sendButton).toBeDisabled();
		await userEvent.type(input, "Write tests");
		expect(sendButton).toBeEnabled();
	});

	it("sends input text and clears the field after success", async () => {
		const onSend = vi.fn().mockResolvedValue(undefined);
		renderComponent(<AgentChatInput {...buildProps({ onSend })} />);

		const input = screen.getByPlaceholderText("Type a message...");
		await userEvent.type(input, "Run focused tests");
		await userEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => {
			expect(onSend).toHaveBeenCalledWith("Run focused tests");
		});
		expect(input).toHaveValue("");
	});

	it("keeps send disabled when the input is disabled or models are unavailable", () => {
		const { unmount } = renderComponent(
			<AgentChatInput
				{...buildProps({ isDisabled: true, initialValue: "Should not send" })}
			/>,
		);
		expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
		unmount();

		renderComponent(
			<AgentChatInput
				{...buildProps({
					isDisabled: false,
					hasModelOptions: false,
					initialValue: "Model required",
				})}
			/>,
		);
		expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
	});

	it("shows a loading spinner while a send is pending", () => {
		renderComponent(
			<AgentChatInput
				{...buildProps({
					isDisabled: true,
					isLoading: true,
					initialValue: "Sending...",
				})}
			/>,
		);

		const sendButton = screen.getByRole("button", { name: "Send" });
		expect(sendButton).toBeDisabled();
		expect(sendButton.querySelector(".animate-spin")).toBeTruthy();
	});
});
