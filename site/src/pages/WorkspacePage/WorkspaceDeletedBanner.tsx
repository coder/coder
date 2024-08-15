import Button from "@mui/material/Button";
import { Alert } from "components/Alert/Alert";
import type { FC } from "react";

export interface WorkspaceDeletedBannerProps {
	handleClick: () => void;
}

export const WorkspaceDeletedBanner: FC<WorkspaceDeletedBannerProps> = ({
	handleClick,
}) => {
	const NewWorkspaceButton = (
		<Button onClick={handleClick} size="small" variant="text">
			Create new workspace
		</Button>
	);

	return (
		<Alert severity="warning" actions={NewWorkspaceButton}>
			This workspace has been deleted and cannot be edited.
		</Alert>
	);
};
