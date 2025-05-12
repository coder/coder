import { MissingBuildParameters } from "api/api";
import { changeVersion } from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import {
	EllipsisVertical,
	SettingsIcon,
	HistoryIcon,
	TrashIcon,
	CopyIcon,
	DownloadIcon,
} from "lucide-react";
import { UpdateBuildParametersDialog } from "pages/WorkspacePage/UpdateBuildParametersDialog";
import { DownloadLogsDialog } from "pages/WorkspacePage/WorkspaceActions/DownloadLogsDialog";
import { useState, type FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router-dom";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";

type WorkspaceMoreActionsProps = {
	workspace: Workspace;
	isDuplicationReady: boolean;
	disabled?: boolean;
	permissions?: {
		changeWorkspaceVersion?: boolean;
	};
	onDuplicate: () => void;
	onDelete: () => void;
};

export const WorkspaceMoreActions: FC<WorkspaceMoreActionsProps> = ({
	workspace,
	disabled,
	permissions,
	isDuplicationReady,
	onDuplicate,
	onDelete,
}) => {
	const queryClient = useQueryClient();

	// Download logs
	const [isDownloadDialogOpen, setIsDownloadDialogOpen] = useState(false);

	// Change version
	const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
	const changeVersionMutation = useMutation(
		changeVersion(workspace, queryClient),
	);

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						size="icon-lg"
						variant="subtle"
						data-testid="workspace-options-button"
						aria-controls="workspace-options"
						disabled={disabled}
					>
						<EllipsisVertical aria-hidden="true" />
						<span className="sr-only">Workspace actions</span>
					</Button>
				</DropdownMenuTrigger>

				<DropdownMenuContent id="workspace-options" align="end">
					<DropdownMenuItem asChild>
						<RouterLink to="settings">
							<SettingsIcon />
							Settings
						</RouterLink>
					</DropdownMenuItem>

					{permissions?.changeWorkspaceVersion && (
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
						onClick={onDuplicate}
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
						onClick={onDelete}
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

			<UpdateBuildParametersDialog
				missedParameters={
					isMissingBuildParameters(changeVersionMutation.error)
						? changeVersionMutation.error.parameters
						: []
				}
				open={isMissingBuildParameters(changeVersionMutation.error)}
				onClose={() => {
					changeVersionMutation.reset();
				}}
				onUpdate={(buildParameters) => {
					if (isMissingBuildParameters(changeVersionMutation.error)) {
						changeVersionMutation.mutate({
							versionId: changeVersionMutation.error.versionId,
							buildParameters,
						});
					}
				}}
			/>

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
		</>
	);
};

const isMissingBuildParameters = (e: unknown): e is MissingBuildParameters => {
	return Boolean(e && e instanceof MissingBuildParameters);
};
