import {
	CopyIcon,
	DownloadIcon,
	EllipsisVerticalIcon,
	HistoryIcon,
	RotateCcwIcon,
	SettingsIcon,
	SquareIcon,
	TrashIcon,
} from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router";
import { ParameterValidationError } from "#/api/api";
import {
	type ApiError,
	getErrorDetail,
	getErrorMessage,
	isApiError,
} from "#/api/errors";
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
import { toast } from "#/components/Toaster/toast";
import { WorkspaceErrorDialog } from "../ErrorDialog/WorkspaceErrorDialog";
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
	onRestart?: () => void;
	isRestarting?: boolean;
	onActionSuccess?: () => Promise<void> | void;
};

export const WorkspaceMoreActions: React.FC<WorkspaceMoreActionsProps> = ({
	workspace,
	disabled,
	onStop,
	isStopping,
	onRestart,
	isRestarting,
	onActionSuccess,
}) => {
	const queryClient = useQueryClient();

	const [workspaceErrorDialog, setWorkspaceErrorDialog] = useState<{
		open: boolean;
		error?: ApiError;
	}>({ open: false });

	// Permissions
	const { data: permissions } = useQuery(workspacePermissions(workspace));

	// Download logs
	const [isDownloadDialogOpen, setIsDownloadDialogOpen] = useState(false);

	// Change version
	const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
	const changeVersionMutation = useMutation(
		changeVersion(workspace, queryClient),
	);

	const handleError = (error: unknown) => {
		if (isApiError(error) && error.code === "ERR_BAD_REQUEST") {
			setWorkspaceErrorDialog({
				open: true,
				error: error,
			});
		} else {
			toast.error(
				getErrorMessage(
					error,
					`Failed to delete workspace "${workspace.name}".`,
				),
				{
					description: getErrorDetail(error),
				},
			);
		}
	};

	// Delete
	const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
	const deleteWorkspaceOptions = deleteWorkspace(workspace, queryClient);
	const deleteWorkspaceMutation = useMutation({
		...deleteWorkspaceOptions,
		onSuccess: async (build) => {
			await deleteWorkspaceOptions.onSuccess?.(build);
			await onActionSuccess?.();
		},
		onError: (error: unknown) => {
			handleError(error);
		},
	});

	// Duplicate
	const { duplicateWorkspace, isDuplicationReady } =
		useWorkspaceDuplication(workspace);

	// The menu closes when a new build starts, because its actions may no
	// longer apply. The workspace data can lag behind an action, so the menu
	// may have been opened before the build arrived.
	// Reference: https://github.com/coder/coder/pull/17775#discussion_r2087273706
	const [openedForBuildId, setOpenedForBuildId] = useState<string>();
	const open = openedForBuildId === workspace.latest_build.id;

	return (
		<>
			<DropdownMenu
				open={open}
				onOpenChange={(nextOpen) => {
					setOpenedForBuildId(nextOpen ? workspace.latest_build.id : undefined);
				}}
			>
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

					{onRestart && (
						<DropdownMenuItem onClick={onRestart} disabled={isRestarting}>
							<RotateCcwIcon />
							Restart&hellip;
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
				onCancel={() => {
					setIsConfirmingDelete(false);
				}}
				onConfirm={(orphan) => {
					deleteWorkspaceMutation.mutate({ orphan });
					setIsConfirmingDelete(false);
				}}
			/>

			<WorkspaceErrorDialog
				open={workspaceErrorDialog.open}
				error={workspaceErrorDialog.error}
				onClose={() => setWorkspaceErrorDialog({ open: false })}
				workspaceOwner={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
				isDeleting
			/>
		</>
	);
};
