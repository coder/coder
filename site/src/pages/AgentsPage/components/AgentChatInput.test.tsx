import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, createRef, type ReactNode } from "react";
import { toast } from "sonner";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import {
	MockChatQueuedMessage,
	MockChatQueuedMessageUnderEdit,
} from "#/testHelpers/chatEntities";
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

	it.each([
		["sends the queue head when nothing is under edit", {}, true],
		[
			"does not send the queue head while the composer edits a message",
			{ editingKind: "queued" },
			false,
		],
		[
			"does not send a queue head the server marks as under edit",
			{ queuedMessages: [MockChatQueuedMessageUnderEdit] },
			false,
		],
		[
			"does not send a queue head whose begin request is pending",
			{ queuedMessageUnderEditID: MockChatQueuedMessage.id },
			false,
		],
	] satisfies Array<
		[string, Partial<ComponentProps<typeof AgentChatInput>>, boolean]
	>)("Enter with an empty composer %s", async (_name, props, sendsHead) => {
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
				queuedMessages={[MockChatQueuedMessage]}
				onPromoteQueuedMessage={onPromoteQueuedMessage}
				{...props}
			/>,
		);

		// Plain Enter sends only once the shortcut preference has loaded.
		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: /^(Send|Save Edit)$/ }),
			).toHaveAttribute("aria-keyshortcuts", "Enter");
		});
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{Enter}");
		if (sendsHead) {
			expect(onPromoteQueuedMessage).toHaveBeenCalledWith(
				MockChatQueuedMessage.id,
			);
		} else {
			expect(onPromoteQueuedMessage).not.toHaveBeenCalled();
		}
	});
});
