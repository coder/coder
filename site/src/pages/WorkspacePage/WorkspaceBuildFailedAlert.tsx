import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC } from "react";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { buildDebugWorkspaceBuildPath } from "#/pages/AgentsPage/utils/workspaceBuildDebug";

type WorkspaceBuildFailedAlertProps = {
	build: WorkspaceBuild;
};

export const WorkspaceBuildFailedAlert: FC<WorkspaceBuildFailedAlertProps> = ({
	build,
}) => {
	const { permissions } = useAuthenticated();

	return (
		<Alert
			severity="error"
			prominent
			actions={
				permissions.createChat && (
					<Button asChild variant="outline" size="sm">
						<a
							href={buildDebugWorkspaceBuildPath(build.id)}
							target="_blank"
							rel="noopener"
						>
							Debug with Coder Agents
							<SquareArrowOutUpRightIcon />
						</a>
					</Button>
				)
			}
		>
			<AlertTitle>Workspace build failed</AlertTitle>
			<AlertDescription>{build.job.error}</AlertDescription>
		</Alert>
	);
};
