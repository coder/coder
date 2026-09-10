import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, type ReactNode } from "react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { MockHeldChatQueuedMessage } from "#/testHelpers/chatEntities";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ organizations: [] }),
}));

const modelOptions = [
	{
		id: "model-config-1",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const renderInput = (children: ReactNode) => {
	return render(<AppProviders>{children}</AppProviders>);
};

beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

describe("AgentChatInput", () => {
	it("accepts drafts without sending while submission is disabled", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={onSend}
				inputRef={inputRef}
				isDisabled
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Draft while models load");
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("Draft while models load");
		});
		await user.keyboard("{Enter}");
		expect(onSend).not.toHaveBeenCalled();
	});

	it("cancels a queued message edit on Escape", async () => {
		const user = userEvent.setup();
		const onCancelHistoryEdit = vi.fn();
		const onEditQueuedMessage = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				queuedMessages={[MockHeldChatQueuedMessage]}
				onEditQueuedMessage={onEditQueuedMessage}
				editingKind="queued"
				onCancelHistoryEdit={onCancelHistoryEdit}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Edit" }));
		expect(onEditQueuedMessage).toHaveBeenCalledWith(
			MockHeldChatQueuedMessage.id,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{Escape}");
		expect(onCancelHistoryEdit).toHaveBeenCalledTimes(1);
	});

	it("does not promote a held queue head on Enter with an empty composer", async () => {
		const user = userEvent.setup();
		const onPromoteQueuedMessage = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				queuedMessages={[MockHeldChatQueuedMessage]}
				onPromoteQueuedMessage={onPromoteQueuedMessage}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{Enter}");
		expect(onPromoteQueuedMessage).not.toHaveBeenCalled();
	});
});
