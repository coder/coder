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
import { useState, type FC } from "react";
import { Link as RouterLink } from "react-router-dom";

type WorkspaceMoreActionsProps = {
	isDuplicationReady: boolean;
	disabled?: boolean;
	onDuplicate: () => void;
	onDelete: () => void;
	onChangeVersion?: () => void;
	permissions?: {
		changeWorkspaceVersion?: boolean;
	};
};

export const WorkspaceMoreActions: FC<WorkspaceMoreActionsProps> = ({
	disabled,
	isDuplicationReady,
	onDuplicate,
	onDelete,
	onChangeVersion,
	permissions,
}) => {
	// Download logs
	const [isDownloadDialogOpen, setIsDownloadDialogOpen] = useState(false);
	const canChangeVersion = permissions?.changeWorkspaceVersion !== false;

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

					{onChangeVersion && canChangeVersion && (
						<DropdownMenuItem onClick={onChangeVersion}>
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
		</>
	);
};
