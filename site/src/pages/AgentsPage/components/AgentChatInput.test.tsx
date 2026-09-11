import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, type ReactNode, useState } from "react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { MockWorkspace } from "#/testHelpers/entities";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";
import type { Harness } from "./HarnessPicker";

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
});

describe("harness prototype", () => {
	it.each<Harness>(["Coder Agents", "Codex", "Claude Code", "Pi"])(
		"selects %s without sending a message",
		async (harness) => {
			const user = userEvent.setup();
			const onHarnessChange = vi.fn();
			const onSend = vi.fn();
			renderInput(
				<AgentChatInput
					onSend={onSend}
					isDisabled={false}
					isLoading={false}
					selectedModel={modelOptions[0].id}
					onModelChange={vi.fn()}
					modelOptions={modelOptions}
					modelSelectorPlaceholder="Select model"
					hasModelOptions
					canConfigureAgentSetup={false}
					selectedHarness="Coder Agents"
					onHarnessChange={onHarnessChange}
				/>,
			);
			await user.click(screen.getByRole("button", { name: "More options" }));
			await user.click(screen.getByRole("button", { name: "Change harness" }));
			await user.click(screen.getByRole("option", { name: harness }));
			expect(onHarnessChange).toHaveBeenCalledWith(harness);
			await user.click(screen.getByRole("button", { name: "More options" }));
			await user.click(screen.getByRole("button", { name: "Change harness" }));
			await user.click(screen.getByRole("option", { name: "Codex" }));
			expect(onHarnessChange).toHaveBeenCalledTimes(2);
			expect(onHarnessChange).toHaveBeenLastCalledWith("Codex");
			expect(onSend).not.toHaveBeenCalled();
		},
	);
	it("selects a harness with the keyboard", async () => {
		const user = userEvent.setup();
		const onHarnessChange = vi.fn();
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
				selectedHarness="Coder Agents"
				onHarnessChange={onHarnessChange}
			/>,
		);
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Change harness" }));
		await user.keyboard("{ArrowDown}{Enter}");
		expect(onHarnessChange).toHaveBeenCalledWith("Codex");
	});
	it("resets to Coder Agents when the harness tile is removed", async () => {
		const user = userEvent.setup();
		const onHarnessChange = vi.fn();
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
				selectedHarness="Codex"
				onHarnessChange={onHarnessChange}
			/>,
		);
		await user.click(
			screen.getByRole("button", { name: "Remove Codex harness" }),
		);
		expect(onHarnessChange).toHaveBeenCalledWith("Coder Agents");
	});
});

describe("custom harness workspace requirement", () => {
	it.each<Harness>(["Codex", "Claude Code", "Pi"])(
		"blocks %s without a workspace or template",
		async (harness) => {
			const user = userEvent.setup();
			const onSend = vi.fn();
			renderInput(
				<AgentChatInput
					onSend={onSend}
					isDisabled={false}
					isLoading={false}
					selectedModel={modelOptions[0].id}
					onModelChange={vi.fn()}
					modelOptions={modelOptions}
					modelSelectorPlaceholder="Select model"
					hasModelOptions
					canConfigureAgentSetup={false}
					selectedHarness={harness}
					initialValue="Build an app"
				/>,
			);
			await user.click(screen.getByRole("textbox", { name: "Chat message" }));
			await user.keyboard("{Enter}");
			expect(onSend).not.toHaveBeenCalled();
		},
	);
	it("replaces workspace and template attachments and requires one before sending", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();
		const onWorkspaceChange = vi.fn();
		const Input = () => {
			const [workspaceId, setWorkspaceId] = useState<string | null>(
				MockWorkspace.id,
			);
			return (
				<AgentChatInput
					onSend={onSend}
					isDisabled={false}
					isLoading={false}
					selectedModel={modelOptions[0].id}
					onModelChange={vi.fn()}
					modelOptions={modelOptions}
					modelSelectorPlaceholder="Select model"
					hasModelOptions
					canConfigureAgentSetup={false}
					selectedHarness="Codex"
					initialValue="Build an app"
					workspaceOptions={[MockWorkspace]}
					selectedWorkspaceId={workspaceId}
					onWorkspaceChange={(id) => {
						setWorkspaceId(id);
						onWorkspaceChange(id);
					}}
				/>
			);
		};
		renderInput(<Input />);
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Attach template" }));
		await user.click(screen.getByRole("option", { name: "Docker" }));
		expect(onWorkspaceChange).toHaveBeenLastCalledWith(null);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledWith("Build an app");
		onSend.mockClear();
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Attach workspace" }));
		await user.click(screen.getByRole("option", { name: MockWorkspace.name }));
		expect(onWorkspaceChange).toHaveBeenLastCalledWith(MockWorkspace.id);
		await user.click(
			screen.getByRole("button", {
				name: `Remove workspace ${MockWorkspace.name}`,
			}),
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{Enter}");
		expect(onSend).not.toHaveBeenCalled();
	});
});
