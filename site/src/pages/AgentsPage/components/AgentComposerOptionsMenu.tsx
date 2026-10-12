import { cn } from "cn";
import { CheckIcon, PaperclipIcon, PencilIcon, PlusIcon } from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { disconnectMCPServerOAuth2 } from "#/api/queries/chats";
import type { MCPServerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";
import { isBelowMdViewport } from "#/utils/mobile";
import { useMCPOAuthFlow } from "../hooks/useMCPOAuthFlow";
import { useAgentComposer } from "./AgentComposer/context";
import { FrameEventBoundary } from "./AgentComposer/Frame";
import {
	enabledMcpServers,
	MCPServerMenuItem,
	setMCPServerSelected,
} from "./AgentComposerMCPMenu";
import { useAgentComposerOptions } from "./AgentComposerOptionsContext";
import {
	AgentComposerWorkspaceMenuEntry,
	AgentComposerWorkspacePicker,
	AgentComposerWorkspaceView,
} from "./AgentComposerWorkspacePicker";

/** Keeps the file input, OAuth, and disconnect state alive independently of the menu's portaled content. */
export const AgentComposerOptionsMenu = () => {
	const options = useAgentComposerOptions();
	const composer = useAgentComposer();
	const { mcp, workspaceSelection, isViewOnly } = options;
	const { state, actions } = composer;
	const mcpServers = enabledMcpServers(mcp);

	const fileInputRef = useRef<HTMLInputElement>(null);
	const [open, setOpen] = useState(false);
	const [view, setView] = useState<"main" | "workspace">("main");
	// Lives at the menu root so the desktop flyout keeps its open state
	// while the options menu content unmounts.
	const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false);
	const [disconnectTarget, setDisconnectTarget] =
		useState<MCPServerConfig | null>(null);

	const queryClient = useQueryClient();
	const disconnectMutation = useMutation(
		disconnectMCPServerOAuth2(queryClient),
	);

	const { connectingServerId, connect } = useMCPOAuthFlow({
		organizationId: options.organizationId,
		onAuthComplete: mcp?.onAuthComplete,
		onFlowSuccess: (serverId) => {
			if (!mcp) {
				return;
			}

			const isEnabledServer = mcpServers.some(
				(server) => server.id === serverId,
			);
			const isSelected = mcp.selectedServerIds.includes(serverId);

			if (isEnabledServer && !isSelected) {
				setMCPServerSelected(mcp, serverId, true);
			}
		},
	});

	const onWorkspaceChange = workspaceSelection?.onChange;
	const hasWorkspaceOptions = workspaceSelection?.options !== undefined;
	const isWorkspaceChoiceOffered =
		hasWorkspaceOptions && onWorkspaceChange !== undefined;
	const isWorkspaceSelectionLoading = workspaceSelection?.isLoading === true;
	const canChangeWorkspace =
		onWorkspaceChange !== undefined && !isWorkspaceSelectionLoading;

	const selectWorkspace = (id: string | null) => {
		onWorkspaceChange?.(id);
		setOpen(false);
	};

	const openFilePicker = () => {
		actions.resetPromptCycle();
		setOpen(false);
		fileInputRef.current?.click();
	};

	const togglePlanning = () => {
		options.planning.onChange(!options.planning.enabled);
		setOpen(false);
	};

	const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
		if (event.target.files?.length) {
			actions.attachFiles(Array.from(event.target.files));
		}
		// Reset so the same file can be selected again.
		event.target.value = "";
	};

	const confirmDisconnect = () => {
		if (!disconnectTarget) {
			return;
		}

		const name = disconnectTarget.display_name;
		disconnectMutation.mutate(disconnectTarget.id, {
			onSuccess: (response) => {
				setDisconnectTarget(null);

				if (response.token_revocation_error) {
					toast.warning(`Disconnected ${name}.`, {
						description: response.token_revocation_error,
					});
				} else {
					toast.success(`Disconnected ${name}.`);
				}
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, `Failed to disconnect ${name}.`));
			},
		});
	};

	const menuHasEnabledItems =
		!state.isDisabled || state.needsSetup || canChangeWorkspace;
	let workspaceEntry: React.ReactNode;

	if (isWorkspaceChoiceOffered) {
		if (isBelowMdViewport()) {
			workspaceEntry = (
				<AgentComposerWorkspaceMenuEntry
					disabled={!canChangeWorkspace}
					onClick={() => setView("workspace")}
				/>
			);
		} else {
			workspaceEntry = (
				<AgentComposerWorkspacePicker
					open={workspacePickerOpen}
					onOpenChange={setWorkspacePickerOpen}
					disabled={!canChangeWorkspace}
					onSelect={(id) => {
						selectWorkspace(id);
						setWorkspacePickerOpen(false);
					}}
				/>
			);
		}
	}

	return (
		<>
			<Popover
				modal={false}
				open={open}
				onOpenChange={(next) => {
					setOpen(next);

					if (!next) {
						setView("main");
					}
				}}
			>
				<PopoverTrigger asChild>
					<Button
						type="button"
						variant="subtle"
						size="icon"
						className={cn(
							"size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0",
							isViewOnly && "hidden",
						)}
						disabled={!menuHasEnabledItems}
						aria-label="More options"
					>
						<PlusIcon />
					</Button>
				</PopoverTrigger>
				<PopoverContent
					side="bottom"
					align="start"
					className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-auto min-w-[200px] p-1"
				>
					{view === "workspace" ? (
						<div className="p-0">
							<AgentComposerWorkspaceView
								onSelect={selectWorkspace}
								onBack={() => setView("main")}
							/>
						</div>
					) : (
						<>
							{state.canAttachFiles && (
								<button
									type="button"
									onClick={openFilePicker}
									className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
								>
									<PaperclipIcon className="size-3.5 shrink-0" />
									Attach file
								</button>
							)}
							<button
								type="button"
								role="menuitemcheckbox"
								aria-checked={options.planning.enabled}
								onClick={togglePlanning}
								disabled={state.isDisabled}
								className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
							>
								<PencilIcon className="size-3.5 shrink-0" />
								<span>Plan first</span>
								{options.planning.enabled && (
									<CheckIcon className="ml-auto size-icon-sm shrink-0" />
								)}
							</button>
							{workspaceEntry}
							{mcp && mcpServers.length > 0 && (
								<>
									<Separator className="my-1" />
									{mcpServers.map((server) => (
										<MCPServerMenuItem
											key={server.id}
											server={server}
											mcp={mcp}
											connectingServerId={connectingServerId}
											isDisabled={state.isDisabled}
											onConnect={connect}
											onDisconnect={(target) => {
												setOpen(false);
												setDisconnectTarget(target);
											}}
										/>
									))}
								</>
							)}
						</>
					)}
				</PopoverContent>
			</Popover>
			{/* Every file type stays selectable: iOS silently greys out
			filtered types, so actions.attachFiles explains any refusal. */}
			{state.canAttachFiles && (
				<input
					ref={fileInputRef}
					type="file"
					data-testid="chat-attachment-file-input"
					multiple
					onChange={handleFileSelect}
					className="hidden"
				/>
			)}
			<FrameEventBoundary className="hidden">
				<ConfirmDialog
					open={disconnectTarget !== null}
					title={`Disconnect ${disconnectTarget?.display_name ?? "MCP server"}?`}
					description="This removes your credentials for this MCP server from Coder. You can authenticate again later."
					type="delete"
					confirmText="Disconnect"
					confirmLoading={disconnectMutation.isPending}
					onConfirm={confirmDisconnect}
					onClose={() => setDisconnectTarget(null)}
				/>
			</FrameEventBoundary>
		</>
	);
};
