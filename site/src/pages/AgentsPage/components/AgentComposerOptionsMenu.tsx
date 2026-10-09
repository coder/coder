import { cn } from "cn";
import {
	CheckIcon,
	type LucideIcon,
	PaperclipIcon,
	PencilIcon,
	PlusIcon,
	ZapIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { disconnectMCPServerOAuth2 } from "#/api/queries/chats";
import type { MCPServerConfig, Workspace } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { isBelowMdViewport } from "#/utils/mobile";
import { useMCPOAuthFlow } from "../hooks/useMCPOAuthFlow";
import { AgentComposerMCPMenu } from "./AgentComposerMCPMenu";
import {
	AgentComposerWorkspacePicker,
	AgentComposerWorkspaceView,
} from "./AgentComposerWorkspacePicker";

type OptionsMenuProps = {
	isDisabled: boolean;
	showAgentSetupNotice: boolean;
	onAttachClick?: () => void;
	planModeEnabled: boolean;
	onPlanModeToggle: (enabled: boolean) => void;
	manageAutomationsEnabled: boolean;
	onManageAutomationsToggle?: (enabled: boolean) => void;
	workspaceOptions?: ReadonlyArray<
		Pick<Workspace, "id" | "name" | "organization_id">
	>;
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (id: string | null) => void;
	chatOrganizationId?: string;
	isWorkspaceLoading?: boolean;
	mcpServers: readonly MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPToggle: (id: string, checked: boolean) => void;
	onMCPAuthComplete?: (id: string) => void;
};

/** Keeps OAuth and disconnect state alive independently of the menu's portaled content. */
export const AgentComposerOptionsMenu = (props: OptionsMenuProps) => {
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
		organizationId: props.chatOrganizationId,
		onAuthComplete: props.onMCPAuthComplete,
		onFlowSuccess: (serverId) => {
			if (
				props.mcpServers.some((server) => server.id === serverId) &&
				!props.selectedMCPServerIds?.includes(serverId)
			) {
				props.onMCPToggle(serverId, true);
			}
		},
	});
	const canUseWorkspacePicker =
		Boolean(props.onWorkspaceChange) && !props.isWorkspaceLoading;
	const selectWorkspace = (id: string | null) => {
		props.onWorkspaceChange?.(id);
		setOpen(false);
	};
	const confirmDisconnect = () => {
		if (!disconnectTarget) return;
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
					if (!next) setView("main");
				}}
			>
				{" "}
				<PopoverTrigger asChild>
					<Button
						type="button"
						variant="subtle"
						size="icon"
						className="size-7 shrink-0 rounded-full [&>svg]:size-icon-sm! [&>svg]:p-0"
						disabled={
							props.isDisabled &&
							!props.showAgentSetupNotice &&
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
					{view === "workspace" ? (
						<AgentComposerWorkspaceView
							workspaceOptions={props.workspaceOptions}
							selectedWorkspaceId={props.selectedWorkspaceId}
							chatOrganizationId={props.chatOrganizationId}
							onSelect={selectWorkspace}
							onBack={() => setView("main")}
						/>
					) : (
						<>
							<ComposerMenuActions {...props} onClose={() => setOpen(false)} />
							{props.workspaceOptions && props.onWorkspaceChange && (
								<AgentComposerWorkspacePicker
									workspaceOptions={props.workspaceOptions}
									selectedWorkspaceId={props.selectedWorkspaceId}
									chatOrganizationId={props.chatOrganizationId}
									onSelect={selectWorkspace}
									isMobile={isBelowMdViewport()}
									open={workspacePickerOpen}
									onOpenChange={setWorkspacePickerOpen}
									disabled={!canUseWorkspacePicker}
									onOpenMobile={() => setView("workspace")}
								/>
							)}
							<AgentComposerMCPMenu
								servers={props.mcpServers}
								selectedServerIds={props.selectedMCPServerIds}
								connectingServerId={connectingServerId}
								isDisabled={props.isDisabled}
								onConnect={connect}
								onToggle={props.onMCPToggle}
								onDisconnect={(server) => {
									setOpen(false);
									setDisconnectTarget(server);
								}}
							/>
						</>
					)}
				</PopoverContent>
			</Popover>
			{/* Portaled dialog keys still bubble through the React composer tree. */}
			<div
				className="contents"
				role="presentation"
				onKeyDown={(event) => {
					if (event.key === "Escape") event.stopPropagation();
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

type MenuActionsProps = Pick<
	OptionsMenuProps,
	| "isDisabled"
	| "onAttachClick"
	| "planModeEnabled"
	| "onPlanModeToggle"
	| "manageAutomationsEnabled"
	| "onManageAutomationsToggle"
> & { onClose: () => void };

const ComposerMenuActions = ({
	isDisabled,
	onAttachClick,
	planModeEnabled,
	onPlanModeToggle,
	manageAutomationsEnabled,
	onManageAutomationsToggle,
	onClose,
}: MenuActionsProps) => (
	<>
		{onAttachClick && (
			<button
				type="button"
				onClick={() => {
					onClose();
					onAttachClick();
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
			checked={planModeEnabled}
			onToggle={() => {
				onPlanModeToggle(!planModeEnabled);
				onClose();
			}}
			disabled={isDisabled}
		/>
		{onManageAutomationsToggle && (
			<MenuCheckboxItem
				icon={ZapIcon}
				label="Manage automations"
				description="Let the agent create and manage automations for you."
				checked={manageAutomationsEnabled}
				onToggle={() => {
					onManageAutomationsToggle(!manageAutomationsEnabled);
					onClose();
				}}
				disabled={isDisabled}
			/>
		)}
	</>
);

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
