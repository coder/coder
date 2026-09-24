import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC } from "react";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	buildDebugWorkspaceBuildPath,
	storeDebugWorkspaceBuildIntent,
} from "#/modules/workspaces/workspaceBuildDebugLink";

type WorkspaceBuildFailedAlertProps = {
	build: WorkspaceBuild;
};

export const WorkspaceBuildFailedAlert: FC<WorkspaceBuildFailedAlertProps> = ({
	build,
}) => {
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
	const canDebugWithAgents =
		experiments.includes("enable-ai-workspace-debug") && permissions.createChat;

	return (
		<Alert
			severity="error"
			prominent
			actions={
				canDebugWithAgents ? (
					<Button asChild size="sm">
						<a
							href={buildDebugWorkspaceBuildPath(build.id)}
							target="_blank"
							rel="noreferrer"
							onClick={() => storeDebugWorkspaceBuildIntent(build.id)}
						>
							Debug with Coder Agents
							<SquareArrowOutUpRightIcon />
						</a>
					</Button>
				) : undefined
			}
		>
			<AlertTitle>Workspace build failed</AlertTitle>
			<AlertDescription>{build.job.error}</AlertDescription>
		</Alert>
	);
};
