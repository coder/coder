import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { getPreferredProxy, ProxyContext } from "#/contexts/ProxyContext";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
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

// WorkspacePill links to the workspace and reads proxy hostnames.
const WorkspacePillProviders = ({
	children,
}: {
	children: React.ReactNode;
}) => (
	<MemoryRouter>
		<ProxyContext.Provider
			value={{
				proxy: getPreferredProxy([], undefined),
				proxies: [],
				proxyLatencies: {},
				latenciesLoaded: true,
				isFetched: true,
				isLoading: false,
				setProxy: vi.fn(),
				clearProxy: vi.fn(),
				refetchProxyLatencies: () => new Date(),
			}}
		>
			{children}
		</ProxyContext.Provider>
	</MemoryRouter>
);

const originalMatchMedia = window.matchMedia;

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("AgentComposerOptions", () => {
	it("attaches files picked from the options menu", async () => {
		const user = userEvent.setup();
		const onAttach = vi.fn();
		renderOptions(<Options />, {
			files: {
				attachments: [],
				onAttach,
				onRemoveAttachment: vi.fn(),
				uploadStates: new Map(),
				previewUrls: new Map(),
				textContents: new Map(),
				workspaceUploads: { uploads: [], onRemove: vi.fn() },
			},
		});
		const input = screen.getByTestId<HTMLInputElement>(
			"chat-attachment-file-input",
		);
		const inputClick = vi.spyOn(input, "click");

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Attach file" }));
		expect(inputClick).toHaveBeenCalledTimes(1);

		const file = new File(["image"], "image.png", { type: "image/png" });
		await user.upload(input, file);
		expect(onAttach).toHaveBeenCalledWith([file]);
		await user.upload(input, file);
		expect(onAttach).toHaveBeenCalledTimes(2);
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
				<Options
					organizationId={workspace.organization_id}
					workspaceSelection={{
						options: [workspace],
						selectedId: null,
						onChange: onWorkspaceChange,
					}}
				/>,
				{ isDisabled: true },
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
		"removes the selected workspace while disabled unless selection is loading (loading: %s)",
		async (isLoading) => {
			const user = userEvent.setup();
			const onWorkspaceChange = vi.fn();
			renderOptions(
				<Options
					workspaceSelection={{
						options: [workspace],
						selectedId: workspace.id,
						onChange: onWorkspaceChange,
						isLoading,
					}}
				/>,
				{ isDisabled: true },
			);

			await user.click(
				screen.getByRole("button", {
					name: `Remove workspace ${workspace.name}`,
				}),
			);
			expect(onWorkspaceChange.mock.calls).toEqual(isLoading ? [] : [[null]]);
		},
	);

	it("detaches the linked workspace while disabled", async () => {
		const user = userEvent.setup();
		const onWorkspaceChange = vi.fn();
		renderOptions(
			<WorkspacePillProviders>
				<Options
					linkedWorkspace={{
						workspace,
						agent: MockWorkspaceAgent,
						chatId: "chat-1",
					}}
					workspaceSelection={{
						options: [workspace],
						selectedId: workspace.id,
						onChange: onWorkspaceChange,
					}}
				/>
			</WorkspacePillProviders>,
			{ isDisabled: true },
		);

		await user.click(
			screen.getByRole("button", { name: `${workspace.name} workspace menu` }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Detach workspace" }),
		);
		expect(onWorkspaceChange).toHaveBeenCalledWith(null);
	});

	it.each([false, true])(
		"dismisses disconnect confirmation and plus menu without composer Escape actions (history editing: %s)",
		async (isEditingHistoryMessage) => {
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
					isEditingHistoryMessage,
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
			if (isEditingHistoryMessage) {
				expect(onCancelHistoryEdit).toHaveBeenCalledTimes(1);
				expect(onInterrupt).not.toHaveBeenCalled();
			} else {
				expect(onInterrupt).toHaveBeenCalledTimes(1);
				expect(onCancelHistoryEdit).not.toHaveBeenCalled();
			}
		},
	);

	it("shares plan toggles with the measured planning badge", async () => {
		const user = userEvent.setup();
		const onPlanModeToggle = vi.fn();
		const ControlledOptions = () => {
			const [planModeEnabled, setPlanModeEnabled] = useState(false);

			return (
				<Options
					planning={{
						enabled: planModeEnabled,
						onChange: (enabled) => {
							setPlanModeEnabled(enabled);
							onPlanModeToggle(enabled);
						},
					}}
				/>
			);
		};
		renderOptions(<ControlledOptions />);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			screen.getByRole("menuitemcheckbox", { name: "Plan first" }),
		);
		await user.click(screen.getByRole("button", { name: "Disable plan mode" }));
		expect(onPlanModeToggle.mock.calls).toEqual([[true], [false]]);
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
