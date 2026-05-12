import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

interface WorkspaceDeletedBannerProps {
	createWorkspaceLink?: string;
	templateName?: string;
}

export const WorkspaceDeletedBanner: FC<WorkspaceDeletedBannerProps> = ({
	createWorkspaceLink,
	templateName,
}) => {
	const canRecreateWorkspace =
		createWorkspaceLink !== undefined && templateName !== undefined;
	const actions = (
		<>
			{canRecreateWorkspace && (
				<Button asChild size="sm">
					<RouterLink to={createWorkspaceLink}>
						Create another from {templateName}
					</RouterLink>
				</Button>
			)}
			<Button
				asChild
				size="sm"
				variant={canRecreateWorkspace ? "outline" : "default"}
				className={
					canRecreateWorkspace
						? "border-border-secondary bg-surface-primary hover:bg-surface-secondary"
						: undefined
				}
			>
				<RouterLink to="/workspaces">Back to workspaces</RouterLink>
			</Button>
		</>
	);

	return (
		<Alert severity="warning" prominent actions={actions}>
			This workspace has been deleted.
		</Alert>
	);
};
