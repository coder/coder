import { cn } from "cn";
import {
	CheckIcon,
	type LucideIcon,
	PaperclipIcon,
	PencilIcon,
	PlusIcon,
	ZapIcon,
} from "lucide-react";
import { createContext, use, useId, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { disconnectMCPServerOAuth2 } from "#/api/queries/chats";
import type {
	MCPServerConfig,
	Workspace,
	WorkspaceAgent,
} from "#/api/typesGenerated";
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
import { useAgentComposer } from "./AgentComposer";
import type {
	AttachedWorkspaceInfo,
	ToolBadgeData,
} from "./AgentComposerBadges";
import { MCPServerMenuItem } from "./AgentComposerMCPMenu";
import {
	AgentComposerWorkspacePicker,
	AgentComposerWorkspaceView,
} from "./AgentComposerWorkspacePicker";

/** Controlled inputs for independently configurable composer options. */
export type AgentComposerOptionsData = {
	organizationId?: string;

	planning: {
		enabled: boolean;
		onChange: (enabled: boolean) => void;
	};

	automations?: {
		enabled: boolean;
		onChange: (enabled: boolean) => void;
	};

	mcp?: {
		servers: readonly MCPServerConfig[];
		selectedServerIds: readonly string[];
		onSelectionChange: (ids: string[]) => void;
		onAuthComplete?: (id: string) => void;
	};

	workspaceSelection?: {
		options: ReadonlyArray<Pick<Workspace, "id" | "name" | "organization_id">>;
		selectedId: string | null;
		onChange?: (id: string | null) => void;
		isLoading?: boolean;
	};

	linkedWorkspace?: {
		workspace?: Workspace;
		agent?: WorkspaceAgent;
		chatId?: string;
		sshCommand?: string;
		attachedWorkspace?: AttachedWorkspaceInfo;
		folder?: string;
	};
};

/** Shared options contract for sibling tool controls. */
type AgentComposerOptionsContextValue = {
	state: Omit<AgentComposerOptionsData, "linkedWorkspace">;
	actions: {
		toggleMcp: (id: string, checked: boolean) => void;
		removeWorkspace?: () => void;
		disablePlanMode: () => void;
	};
	meta: {
		badges: readonly ToolBadgeData[];
	};
};

export const OptionsContext =
	createContext<AgentComposerOptionsContextValue | null>(null);

/** Reads shared tool state inside AgentComposerOptions.Provider. */
export function useAgentComposerOptions() {
	const context = use(OptionsContext);

	if (!context) {
		throw new Error(
			"useAgentComposerOptions must be used inside AgentComposerOptions.Provider",
		);
	}

	return context;
}

/** Keeps OAuth and disconnect state alive independently of the menu's portaled content. */
export const AgentComposerOptionsMenu = ({
	showAgentSetupNotice = false,
}: {
	showAgentSetupNotice?: boolean;
}) => {
	const options = useAgentComposerOptions();
	const composer = useAgentComposer();
	const { mcp, workspaceSelection } = options.state;

	const [open, setOpen] = useState(false);
	const [view, setView] = useState<"main" | "workspace">("main");
	const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false);
	const [disconnectTarget, setDisconnectTarget] =
		useState<MCPServerConfig | null>(null);

	const queryClient = useQueryClient();
	const disconnectMutation = useMutation(
		disconnectMCPServerOAuth2(queryClient),
	);

	const { connectingServerId, connect } = useMCPOAuthFlow({
		organizationId: options.state.organizationId,
		onAuthComplete: mcp?.onAuthComplete,
		onFlowSuccess: (serverId) => {
			if (
				mcp?.servers.some((server) => server.id === serverId) &&
				!mcp.selectedServerIds.includes(serverId)
			) {
				options.actions.toggleMcp(serverId, true);
			}
		},
	});

	const canUseWorkspacePicker =
		workspaceSelection?.onChange !== undefined && !workspaceSelection.isLoading;

	const selectWorkspace = (id: string | null) => {
		workspaceSelection?.onChange?.(id);
		setOpen(false);
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
			onError: (error) =>
				toast.error(getErrorMessage(error, `Failed to disconnect ${name}.`)),
		});
	};

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
						className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
						disabled={
							composer.state.isDisabled &&
							!showAgentSetupNotice &&
							!canUseWorkspacePicker
						}
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
					{view === "workspace" && workspaceSelection ? (
						<AgentComposerWorkspaceView
							workspaceOptions={workspaceSelection.options}
							selectedWorkspaceId={workspaceSelection.selectedId}
							chatOrganizationId={options.state.organizationId}
							onSelect={selectWorkspace}
							onBack={() => setView("main")}
						/>
					) : (
						<>
							<ComposerMenuActions onClose={() => setOpen(false)} />
							{workspaceSelection?.onChange && (
								<AgentComposerWorkspacePicker
									workspaceOptions={workspaceSelection.options}
									selectedWorkspaceId={workspaceSelection.selectedId}
									chatOrganizationId={options.state.organizationId}
									onSelect={selectWorkspace}
									isMobile={isBelowMdViewport()}
									open={workspacePickerOpen}
									onOpenChange={setWorkspacePickerOpen}
									disabled={!canUseWorkspacePicker}
									onOpenMobile={() => setView("workspace")}
								/>
							)}
							{mcp && mcp.servers.length > 0 && (
								<>
									<Separator className="my-1" />
									{mcp.servers.map((server) => (
										<MCPServerMenuItem
											key={server.id}
											server={server}
											selectedServerIds={mcp.selectedServerIds}
											connectingServerId={connectingServerId}
											isDisabled={composer.state.isDisabled}
											onConnect={connect}
											onToggle={options.actions.toggleMcp}
											onDisconnect={(server) => {
												setOpen(false);
												setDisconnectTarget(server);
											}}
										/>
									))}
								</>
							)}
						</>
					)}
				</PopoverContent>
			</Popover>
			{/* Portaled dialog keys still bubble through the React composer tree. */}
			<div
				className="contents"
				role="presentation"
				onKeyDown={(event) => {
					if (event.key === "Escape") {
						event.stopPropagation();
					}
				}}
			>
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
			</div>
		</>
	);
};

const ComposerMenuActions = ({ onClose }: { onClose: () => void }) => {
	const { state, actions } = useAgentComposer();
	const { planning, automations } = useAgentComposerOptions().state;

	return (
		<>
			{state.canAttachFiles && (
				<button
					type="button"
					onClick={() => {
						onClose();
						actions.openFilePicker();
					}}
					className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
				>
					<PaperclipIcon className="size-3.5 shrink-0" />
					Attach file
				</button>
			)}
			<MenuCheckboxItem
				icon={PencilIcon}
				label="Plan first"
				checked={planning.enabled}
				onToggle={() => {
					planning.onChange(!planning.enabled);
					onClose();
				}}
				disabled={state.isDisabled}
			/>
			{automations && (
				<MenuCheckboxItem
					icon={ZapIcon}
					label="Manage automations"
					description="Let the agent create and manage automations for you."
					checked={automations.enabled}
					onToggle={() => {
						automations.onChange(!automations.enabled);
						onClose();
					}}
					disabled={state.isDisabled}
				/>
			)}
		</>
	);
};

const MenuCheckboxItem = ({
	icon: Icon,
	label,
	description,
	checked,
	onToggle,
	disabled,
}: {
	icon: LucideIcon;
	label: string;
	description?: string;
	checked: boolean;
	onToggle: () => void;
	disabled: boolean;
}) => {
	const id = useId();

	return (
		<button
			type="button"
			role="menuitemcheckbox"
			aria-checked={checked}
			aria-labelledby={`${id}-label`}
			aria-describedby={description ? `${id}-description` : undefined}
			onClick={onToggle}
			disabled={disabled}
			className={cn(
				"flex w-full cursor-pointer gap-1.5 border-none bg-transparent px-1 text-left text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50",
				description ? "items-start py-1.5" : "h-8 items-center",
			)}
		>
			<Icon className={cn("size-3.5 shrink-0", description && "mt-px")} />
			<span className="flex min-w-0 flex-col gap-0.5">
				<span id={`${id}-label`}>{label}</span>
				{description && (
					<span id={`${id}-description`} className="max-w-48 text-2xs">
						{description}
					</span>
				)}
			</span>
			{checked && <CheckIcon className="ml-auto size-icon-sm shrink-0" />}
		</button>
	);
};
