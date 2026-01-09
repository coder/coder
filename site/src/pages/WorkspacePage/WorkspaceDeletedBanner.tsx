import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import type { FC } from "react";

interface WorkspaceDeletedBannerProps {
	handleClick: () => void;
}

export const WorkspaceDeletedBanner: FC<WorkspaceDeletedBannerProps> = ({
	handleClick,
}) => {
	const NewWorkspaceButton = (
		<Button onClick={handleClick} size="sm" variant="subtle">
			Create new workspace
		</Button>
	);

	return (
		<Alert severity="warning" prominent actions={NewWorkspaceButton}>
			This workspace has been deleted and cannot be edited.
		</Alert>
	);
};
