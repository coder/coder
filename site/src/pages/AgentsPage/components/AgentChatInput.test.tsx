import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, type ReactNode } from "react";
import { toast } from "sonner";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { MockChatQueuedMessageUnderEdit } from "#/testHelpers/chatEntities";
import { createMockFile } from "#/testHelpers/files";
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

	it("attaches supported dropped files and reports unsupported ones", () => {
		const onAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const svg = createMockFile("diagram.svg", "image/svg+xml");
		const zip = createMockFile("archive.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [svg, zip] },
		});

		expect(onAttach).toHaveBeenCalledWith([svg]);
		expect(toastError).toHaveBeenCalledWith(
			"Unsupported file type: archive.zip",
		);
	});

	it("does not promote a queue head under edit on Enter with an empty composer", async () => {
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
				queuedMessages={[MockChatQueuedMessageUnderEdit]}
				onPromoteQueuedMessage={onPromoteQueuedMessage}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{Enter}");
		expect(onPromoteQueuedMessage).not.toHaveBeenCalled();
	});
});
