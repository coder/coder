import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { MockWorkspace } from "#/testHelpers/entities";
import { AgentComposer, AgentComposerRuntimeProvider } from "./AgentComposer";
import { AgentComposerOptions } from "./AgentComposerOptions";

const workspace = { ...MockWorkspace, name: "my-workspace" };

const Options = (
	props: Partial<
		Omit<React.ComponentProps<typeof AgentComposerOptions.Provider>, "children">
	>,
) => (
	<AgentComposerOptions.Provider
		planning={{ enabled: false, onChange: vi.fn() }}
		{...props}
	>
		<AgentComposerOptions.Frame>
			<AgentComposerOptions.Menu />
			<AgentComposerOptions.Badges includePlanning />
		</AgentComposerOptions.Frame>
	</AgentComposerOptions.Provider>
);

const renderOptions = (
	children: React.ReactNode,
	bindings: Partial<
		React.ComponentProps<typeof AgentComposerRuntimeProvider>["bindings"]
	> = {},
) => {
	return render(
		<AgentComposerRuntimeProvider
			bindings={{
				onSend: vi.fn(),
				isDisabled: false,
				isLoading: false,
				initialValue: "",
				onContentChange: vi.fn(),
				...bindings,
			}}
		>
			{children}
		</AgentComposerRuntimeProvider>,
		{ wrapper: AppProviders },
	);
};

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("AgentComposerOptions", () => {
	it("blocks workspace changes while pending, not while the composer is disabled", async () => {
		const user = userEvent.setup();
		const onWorkspaceChange = vi.fn();
		const options = (isLoading: boolean) => (
			<Options
				workspaceSelection={{
					options: [workspace],
					selectedId: workspace.id,
					onChange: onWorkspaceChange,
					isLoading,
				}}
			/>
		);
		const content = (isLoading: boolean) => (
			<AgentComposerRuntimeProvider
				bindings={{
					onSend: vi.fn(),
					isDisabled: true,
					isLoading: false,
					initialValue: "",
					onContentChange: vi.fn(),
				}}
			>
				{options(isLoading)}
			</AgentComposerRuntimeProvider>
		);
		const { rerender } = render(content(true), { wrapper: AppProviders });
		await user.click(
			screen.getByRole("button", {
				name: `Remove workspace ${workspace.name}`,
			}),
		);
		expect(onWorkspaceChange).not.toHaveBeenCalled();

		rerender(content(false));
		await user.click(
			screen.getByRole("button", {
				name: `Remove workspace ${workspace.name}`,
			}),
		);
		expect(onWorkspaceChange).toHaveBeenCalledWith(null);
	});

	it("dismisses portaled options without canceling the history edit or interrupting", async () => {
		const user = userEvent.setup();
		const onInterrupt = vi.fn();
		const onCancelHistoryEdit = vi.fn();
		const server: typeof MockMCPServerConfig = {
			...MockMCPServerConfig,
			auth_type: "oauth2",
			auth_connected: true,
		};
		renderOptions(
			<AgentComposer.Frame>
				<Options
					mcp={{
						servers: [server],
						selectedServerIds: [],
						onSelectionChange: vi.fn(),
					}}
				/>
			</AgentComposer.Frame>,
			{
				isStreaming: true,
				onInterrupt,
				isEditingHistoryMessage: true,
				onCancelHistoryEdit,
			},
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

		// Escape that closes the plus menu belongs to the menu. Focus then
		// returns to the trigger inside the composer, where Escape acts.
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.keyboard("{Escape}");
		expect(onInterrupt).not.toHaveBeenCalled();
		expect(onCancelHistoryEdit).not.toHaveBeenCalled();

		await user.keyboard("{Escape}");
		expect(onCancelHistoryEdit).toHaveBeenCalledTimes(1);
		expect(onInterrupt).not.toHaveBeenCalled();
	});

	it("keeps OAuth completion active after the plus menu closes", async () => {
		const user = userEvent.setup();
		const onMCPAuthComplete = vi.fn();
		const onMCPSelectionChange = vi.fn();
		const server: typeof MockMCPServerConfig = {
			...MockMCPServerConfig,
			auth_type: "oauth2",
			auth_connected: false,
		};
		vi.spyOn(window, "open").mockReturnValue(window);
		vi.spyOn(window, "close").mockImplementation(() => {});
		renderOptions(
			<Options
				organizationId="org-1"
				mcp={{
					servers: [server],
					selectedServerIds: [],
					onAuthComplete: onMCPAuthComplete,
					onSelectionChange: onMCPSelectionChange,
				}}
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
