import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, createRef, type ReactNode } from "react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
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

const inputProps = {
	onSend: vi.fn(),
	isDisabled: false,
	isLoading: false,
	selectedModel: modelOptions[0].id,
	onModelChange: vi.fn(),
	modelOptions,
	modelSelectorPlaceholder: "Select model",
	hasModelOptions: true,
	canConfigureAgentSetup: false,
} satisfies ComponentProps<typeof AgentChatInput>;

const mockSentryMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-sentry",
	display_name: "Sentry",
	availability: "force_on",
	auth_type: "oauth2",
	auth_connected: true,
};

const mockLinearMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-linear",
	display_name: "Linear",
	availability: "default_on",
	auth_type: "api_key",
};

const mockGitHubMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	availability: "default_on",
	auth_type: "oauth2",
	auth_connected: true,
};

const mockGitHubMCPNeedingAuth: TypesGen.MCPServerConfig = {
	...mockGitHubMCP,
	auth_connected: false,
};

const mockNotionMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-notion",
	display_name: "Notion",
	availability: "default_on",
	auth_type: "api_key",
};

const mockMCPServers = [mockSentryMCP, mockLinearMCP, mockGitHubMCP];
const mockSelectedMCPServerIds = mockMCPServers.map((server) => server.id);

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

	it("removes a selected MCP server from the group", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={mockMCPServers}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "3 MCPs" }));
		const popover = within(screen.getByRole("dialog"));
		expect(popover.getByText("Always on")).toBeInTheDocument();
		expect(
			popover.queryByRole("button", { name: "Remove Sentry" }),
		).not.toBeInTheDocument();
		await user.click(popover.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockGitHubMCP.id,
		]);
	});

	it("enables an unselected MCP server from the plus menu while the group is collapsed", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[...mockMCPServers, mockNotionMCP]}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			await screen.findByRole("switch", { name: "Enable Notion" }),
		);
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockLinearMCP.id,
			mockGitHubMCP.id,
			mockNotionMCP.id,
		]);
	});

	it("keeps two active MCP servers as individual pills", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockLinearMCP, mockGitHubMCP]}
				selectedMCPServerIds={[mockLinearMCP.id, mockGitHubMCP.id]}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([mockGitHubMCP.id]);
	});

	it("removes a single MCP server directly from the toolbar", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockLinearMCP]}
				selectedMCPServerIds={[mockLinearMCP.id]}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([]);
	});

	it("excludes selected MCP servers that still need OAuth from the group", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockSentryMCP, mockLinearMCP, mockGitHubMCPNeedingAuth]}
				selectedMCPServerIds={[
					mockSentryMCP.id,
					mockLinearMCP.id,
					mockGitHubMCPNeedingAuth.id,
				]}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockGitHubMCPNeedingAuth.id,
		]);
	});

	it("allows viewing a disabled MCP group without changing its selection", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				isDisabled
				mcpServers={mockMCPServers}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "3 MCPs" }));
		await user.click(
			within(screen.getByRole("dialog")).getByRole("button", {
				name: "Remove Linear",
			}),
		);
		expect(onMCPSelectionChange).not.toHaveBeenCalled();
	});
});
