import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
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
import { AgentComposer, AgentComposerProvider } from "./AgentComposer";
import { AgentComposerOptions } from "./AgentComposerOptions";

const modelOptions = MockPersonalModelOptions;
const workspace = { ...MockWorkspace, name: "my-workspace" };

const modelProps = {
	selectedModel: modelOptions[0].id,
	onModelChange: vi.fn(),
	modelOptions,
	modelSelectorPlaceholder: "Select model",
	isModelCatalogLoading: false,
} satisfies React.ComponentProps<typeof AgentComposerOptions.Model>;

const optionsProps = {
	planning: { enabled: false, onChange: vi.fn() },
} satisfies Omit<
	React.ComponentProps<typeof AgentComposerOptions.Provider>,
	"children"
>;

const Options = (
	props: Partial<
		Omit<React.ComponentProps<typeof AgentComposerOptions.Provider>, "children">
	>,
) => (
	<AgentComposerOptions.Provider {...optionsProps} {...props}>
		<AgentComposerOptions.Frame>
			<AgentComposerOptions.Menu />
			<AgentComposerOptions.Model {...modelProps} />
			<AgentComposerOptions.PlanningBadge />
			<AgentComposerOptions.Badges />
		</AgentComposerOptions.Frame>
	</AgentComposerOptions.Provider>
);

const workspaceOptions = [workspace];

const renderOptions = (
	children: React.ReactNode,
	bindings: Partial<
		React.ComponentProps<typeof AgentComposerProvider>["bindings"]
	> = {},
) => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { staleTime: Number.POSITIVE_INFINITY } },
	});
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);
	return render(
		<AppProviders queryClient={queryClient}>
			<AgentComposerProvider
				bindings={{
					onSend: vi.fn(),
					isDisabled: false,
					isLoading: false,
					initialValue: "",
					onContentChange: vi.fn(),
					hasModelOptions: true,
					...bindings,
				}}
			>
				{children}
			</AgentComposerProvider>
		</AppProviders>,
	);
};

const originalMatchMedia = window.matchMedia;

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("AgentComposerOptions", () => {
	it("opens the file picker from the options menu", async () => {
		const user = userEvent.setup();
		renderOptions(
			<>
				<AgentComposer.Attachments />
				<Options />
			</>,
			{ onAttach: vi.fn() },
		);
		const onAttachClick = vi.spyOn(
			screen.getByTestId("chat-attachment-file-input"),
			"click",
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(screen.getByRole("button", { name: "Attach file" }));
		expect(onAttachClick).toHaveBeenCalledTimes(1);
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
						options: workspaceOptions,
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
		"dismisses disconnect confirmation without composer Escape actions (history editing: %s)",
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

	it.each(["standalone", "measured"])(
		"shares plan toggles with the %s planning badge",
		async (placement) => {
			const user = userEvent.setup();
			const onPlanModeToggle = vi.fn();
			const ControlledOptions = () => {
				const [planModeEnabled, setPlanModeEnabled] = useState(false);

				return (
					<AgentComposerOptions.Provider
						planning={{
							enabled: planModeEnabled,
							onChange: (enabled) => {
								setPlanModeEnabled(enabled);
								onPlanModeToggle(enabled);
							},
						}}
					>
						<AgentComposerOptions.Frame>
							<AgentComposerOptions.Menu />
							{placement === "standalone" ? (
								<AgentComposerOptions.PlanningBadge />
							) : (
								<AgentComposerOptions.Badges
									leadingBadges={planModeEnabled ? [{ kind: "planning" }] : []}
								/>
							)}
						</AgentComposerOptions.Frame>
					</AgentComposerOptions.Provider>
				);
			};
			renderOptions(<ControlledOptions />);

			await user.click(screen.getByRole("button", { name: "More options" }));
			await user.click(
				screen.getByRole("menuitemcheckbox", { name: "Plan first" }),
			);
			await user.click(
				screen.getByRole("button", { name: "Disable plan mode" }),
			);
			expect(onPlanModeToggle.mock.calls).toEqual([[true], [false]]);
		},
	);

	it("shares MCP selection between independently composed menu and badges", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		const server: typeof MockMCPServerConfig = {
			...MockMCPServerConfig,
			auth_type: "none",
		};
		const ControlledOptions = () => {
			const [selectedMCPServerIds, setSelectedMCPServerIds] = useState<
				string[]
			>([]);

			return (
				<AgentComposerOptions.Provider
					{...optionsProps}
					mcp={{
						servers: [server],
						selectedServerIds: selectedMCPServerIds,
						onSelectionChange: (ids) => {
							setSelectedMCPServerIds(ids);
							onMCPSelectionChange(ids);
						},
					}}
				>
					<AgentComposerOptions.Menu />
					<AgentComposerOptions.Frame>
						<AgentComposerOptions.Badges />
					</AgentComposerOptions.Frame>
				</AgentComposerOptions.Provider>
			);
		};
		renderOptions(<ControlledOptions />);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			screen.getByRole("switch", { name: `Enable ${server.display_name}` }),
		);
		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			screen.getByRole("button", { name: `Remove ${server.display_name}` }),
		);
		expect(onMCPSelectionChange.mock.calls).toEqual([[[server.id]], [[]]]);
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
