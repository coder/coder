import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { preferenceSettingsKey } from "#/api/queries/users";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { MockPersonalModelOptions } from "#/testHelpers/chatModels";
import {
	MockUserPreferenceSettings,
	MockWorkspace,
} from "#/testHelpers/entities";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
import { AgentComposer } from "./AgentComposer";
import { AgentComposerOptions } from "./AgentComposerOptions";

const modelOptions = MockPersonalModelOptions.map((model) => ({ ...model }));
const workspace = { ...MockWorkspace, name: "my-workspace" };

const optionsProps = {
	isDisabled: false,
	showAgentSetupNotice: false,
	hasContextUsage: false,
	selectedModel: modelOptions[0].id,
	onModelChange: vi.fn(),
	modelOptions,
	modelSelectorPlaceholder: "Select model",
	planModeEnabled: false,
	onPlanModeToggle: vi.fn(),
	isModelCatalogLoading: false,
} satisfies React.ComponentProps<typeof AgentComposerOptions>;

const workspaceOptions = [workspace];

const renderOptions = (children: React.ReactNode) => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { staleTime: Number.POSITIVE_INFINITY } },
	});
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);
	return render(
		<AppProviders queryClient={queryClient}>{children}</AppProviders>,
	);
};

const originalMatchMedia = window.matchMedia;

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("AgentComposerOptions", () => {
	it("delegates attachment and controlled option toggles", async () => {
		const user = userEvent.setup();
		const onAttachClick = vi.fn();
		const onPlanModeToggle = vi.fn();
		const onManageAutomationsToggle = vi.fn();
		renderOptions(
			<AgentComposerOptions
				{...optionsProps}
				onAttachClick={onAttachClick}
				onPlanModeToggle={onPlanModeToggle}
				onManageAutomationsToggle={onManageAutomationsToggle}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Attach file" }));
		expect(onAttachClick).toHaveBeenCalledTimes(1);
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			screen.getByRole("menuitemcheckbox", { name: "Plan first" }),
		);
		expect(onPlanModeToggle).toHaveBeenCalledWith(true);
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			screen.getByRole("menuitemcheckbox", { name: "Manage automations" }),
		);
		expect(onManageAutomationsToggle).toHaveBeenCalledWith(true);
	});

	it.each([false, true])(
		"selects a workspace while disabled (mobile: %s)",
		async (isMobile) => {
			vi.stubGlobal("matchMedia", (query: string) => {
				const result = originalMatchMedia(query);
				return query === belowMdViewportMediaQuery
					? { ...result, matches: isMobile }
					: result;
			});
			const user = userEvent.setup();
			const onWorkspaceChange = vi.fn();
			renderOptions(
				<AgentComposerOptions
					{...optionsProps}
					isDisabled
					workspaceOptions={workspaceOptions}
					chatOrganizationId={workspace.organization_id}
					onWorkspaceChange={onWorkspaceChange}
				/>,
			);

			await user.click(screen.getByRole("button", { name: "More options" }));
			await user.click(
				screen.getByRole("button", { name: "Attach workspace" }),
			);
			await user.click(screen.getByRole("option", { name: workspace.name }));
			expect(onWorkspaceChange).toHaveBeenCalledWith(workspace.id);
		},
	);

	it.each([false, true])(
		"dismisses disconnect confirmation without composer Escape actions (history editing: %s)",
		async (isEditingHistoryMessage) => {
			const user = userEvent.setup();
			const onInterrupt = vi.fn();
			const onCancelHistoryEdit = vi.fn();
			const server = {
				...MockMCPServerConfig,
				auth_type: "oauth2",
				auth_connected: true,
			};
			renderOptions(
				<AgentComposer.Provider
					bindings={{
						onSend: vi.fn(),
						isDisabled: false,
						isLoading: false,
						initialValue: "",
						onContentChange: vi.fn(),
						hasModelOptions: true,
						isStreaming: true,
						onInterrupt,
						isEditingHistoryMessage,
						onCancelHistoryEdit,
					}}
				>
					<AgentComposer.Frame>
						<AgentComposerOptions {...optionsProps} mcpServers={[server]} />
					</AgentComposer.Frame>
				</AgentComposer.Provider>,
			);

			await user.click(screen.getByRole("button", { name: "More options" }));
			await user.click(
				screen.getByRole("button", {
					name: `Disconnect ${server.display_name}`,
				}),
			);
			await screen.findByRole("dialog", {
				name: `Disconnect ${server.display_name}?`,
			});
			await user.keyboard("{Escape}");
			expect(onInterrupt).not.toHaveBeenCalled();
			expect(onCancelHistoryEdit).not.toHaveBeenCalled();

			// The dismissed dialog releases focus, and plus-menu Escape
			// still reaches the composer's existing keyboard handler.
			await user.click(screen.getByRole("button", { name: "More options" }));
			await user.keyboard("{Escape}");
			if (isEditingHistoryMessage) {
				expect(onCancelHistoryEdit).toHaveBeenCalledTimes(1);
				expect(onInterrupt).not.toHaveBeenCalled();
			} else {
				expect(onInterrupt).toHaveBeenCalledTimes(1);
				expect(onCancelHistoryEdit).not.toHaveBeenCalled();
			}
		},
	);

	it("keeps OAuth completion active after the plus menu closes", async () => {
		const user = userEvent.setup();
		const onMCPAuthComplete = vi.fn();
		const onMCPSelectionChange = vi.fn();
		const server = {
			...MockMCPServerConfig,
			auth_type: "oauth2",
			auth_connected: false,
		};
		vi.spyOn(window, "open").mockReturnValue(window);
		vi.spyOn(window, "close").mockImplementation(() => {});
		renderOptions(
			<AgentComposerOptions
				{...optionsProps}
				chatOrganizationId="org-1"
				mcpServers={[server]}
				selectedMCPServerIds={[]}
				onMCPAuthComplete={onMCPAuthComplete}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Auth" }));
		await user.click(screen.getByRole("button", { name: "More options" }));
		act(() => {
			window.dispatchEvent(
				new MessageEvent("message", {
					origin: location.origin,
					source: window,
					data: { type: "mcp-oauth2-complete", serverID: server.id },
				}),
			);
		});

		await waitFor(() => {
			expect(onMCPAuthComplete).toHaveBeenCalledWith(server.id);
			expect(onMCPSelectionChange).toHaveBeenCalledWith([server.id]);
		});
	});
});
