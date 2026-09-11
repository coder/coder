import {
	CopyIcon,
	DownloadIcon,
	EllipsisVerticalIcon,
	HistoryIcon,
	SettingsIcon,
	SquareIcon,
	TrashIcon,
} from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router";
import { ParameterValidationError } from "#/api/api";
import {
	changeVersion,
	deleteWorkspace,
	workspacePermissions,
} from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { UpdateBuildParametersDialog } from "../WorkspaceUpdateDialogs";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";
import { DownloadLogsDialog } from "./DownloadLogsDialog";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

type WorkspaceMoreActionsProps = {
	workspace: Workspace;
	disabled: boolean;
	onStop?: () => void;
	isStopping?: boolean;
	onActionSuccess?: () => Promise<void> | void;
};

export const WorkspaceMoreActions: FC<WorkspaceMoreActionsProps> = ({
	workspace,
	disabled,
	onStop,
	isStopping,
	onActionSuccess,
}) => {
	const queryClient = useQueryClient();

	// Permissions
	const { data: permissions } = useQuery(workspacePermissions(workspace));

	// Download logs
	const [isDownloadDialogOpen, setIsDownloadDialogOpen] = useState(false);

	// Change version
	const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
	const changeVersionMutation = useMutation(
		changeVersion(workspace, queryClient),
	);

	// Delete
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const deleteWorkspaceOptions = deleteWorkspace(workspace, queryClient);
	const deleteWorkspaceMutation = useMutation({
		...deleteWorkspaceOptions,
		onSuccess: async (build) => {
			await deleteWorkspaceOptions.onSuccess?.(build);
			await onActionSuccess?.();
			setIsConfirmingDelete(false);
		},
	});

	// Duplicate
	const { duplicateWorkspace, isDuplicationReady } =
		useWorkspaceDuplication(workspace);

	// Since the workspace state is not updated immediately after the mutation, we
	// need to be sure the menu is closed when the action gets disabled.
	// Reference: https://github.com/coder/coder/pull/17775#discussion_r2087273706
	const [open, setOpen] = useState(false);
	useEffect(() => {
		setOpen((open) => (disabled ? false : open));
	});

	return (
		<>
			<DropdownMenu open={open} onOpenChange={setOpen}>
				<DropdownMenuTrigger asChild>
					<Button
						size="icon-lg"
						variant="subtle"
						data-testid="workspace-options-button"
						aria-controls="workspace-options"
						disabled={disabled}
					>
						<EllipsisVerticalIcon aria-hidden="true" />
						<span className="sr-only">Workspace actions</span>
					</Button>
				</DropdownMenuTrigger>

				<DropdownMenuContent id="workspace-options" align="end">
					{onStop && (
						<DropdownMenuItem onClick={onStop} disabled={isStopping}>
							<SquareIcon />
							Stop&hellip;
						</DropdownMenuItem>
					)}

					<DropdownMenuItem asChild>
						<RouterLink
							to={`/@${workspace.owner_name}/${workspace.name}/settings`}
						>
							<SettingsIcon />
							Settings
						</RouterLink>
					</DropdownMenuItem>

					{permissions?.updateWorkspaceVersion && (
						<DropdownMenuItem
							onClick={() => {
								setChangeVersionDialogOpen(true);
							}}
						>
							<HistoryIcon />
							Change version&hellip;
						</DropdownMenuItem>
					)}

					<DropdownMenuItem
						onClick={duplicateWorkspace}
						disabled={!isDuplicationReady}
					>
						<CopyIcon />
						Duplicate&hellip;
					</DropdownMenuItem>

					<DropdownMenuItem onClick={() => setIsDownloadDialogOpen(true)}>
						<DownloadIcon />
						Download logs&hellip;
					</DropdownMenuItem>

					<DropdownMenuSeparator />

					<DropdownMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onClick={() => {
							setIsConfirmingDelete(true);
						}}
						data-testid="delete-button"
					>
						<TrashIcon />
						Delete&hellip;
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>

			<DownloadLogsDialog
				workspace={workspace}
				open={isDownloadDialogOpen}
				onClose={() => setIsDownloadDialogOpen(false)}
			/>

			{changeVersionMutation.error instanceof ParameterValidationError && (
				<UpdateBuildParametersDialog
					workspace={workspace}
					error={changeVersionMutation.error}
					onClose={() => {
						changeVersionMutation.reset();
					}}
				/>
			)}

			<ChangeWorkspaceVersionDialog
				workspace={workspace}
				open={changeVersionDialogOpen}
				onClose={() => {
					setChangeVersionDialogOpen(false);
				}}
				onConfirm={(version) => {
					setChangeVersionDialogOpen(false);
					changeVersionMutation.mutate({ versionId: version.id });
				}}
			/>

			<WorkspaceDeleteDialog
				workspace={workspace}
				canDeleteFailedWorkspace={Boolean(permissions?.deleteFailedWorkspace)}
				isOpen={isConfirmingDelete}
				confirmLoading={deleteWorkspaceMutation.isPending}
				error={deleteWorkspaceMutation.error}
				onCancel={() => {
					setIsConfirmingDelete(false);
					deleteWorkspaceMutation.reset();
				}}
				onConfirm={(orphan) => {
					deleteWorkspaceMutation.mutate({ orphan });
				}}
			/>
		</>
	);
};
