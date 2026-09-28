import { Link as RouterLink } from "react-router";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

type WorkspaceDeletedBannerProps = {
	createWorkspaceLink: string;
	templateName: string;
};

export const WorkspaceDeletedBanner: React.FC<WorkspaceDeletedBannerProps> = ({
	createWorkspaceLink,
	templateName,
}) => {
	const createWorkspaceButton = (
		<Button asChild size="sm">
			<RouterLink to={createWorkspaceLink}>
				Create another from {templateName}
			</RouterLink>
		</Button>
	);

	return (
		<Alert severity="warning" prominent actions={createWorkspaceButton}>
			This workspace has been deleted.
		</Alert>
	);
};
