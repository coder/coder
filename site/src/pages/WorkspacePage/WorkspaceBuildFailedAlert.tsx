import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { checkAuthorization } from "#/api/queries/authCheck";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { chatPermissionChecks } from "#/modules/permissions/chats";
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
	const { experiments } = useDashboard();
	const debugEnabled = experiments.includes("enable-ai-workspace-debug");
	// The chat is created in the build's organization, so the site-wide
	// "can create a chat somewhere" permission is not enough.
	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: chatPermissionChecks(build.job.organization_id),
		}),
		enabled: debugEnabled,
	});
	const canDebugWithAgents =
		debugEnabled && permissionsQuery.data?.createChatInOrganization === true;

	return (
		<Alert
			severity="error"
			prominent
			actions={
				canDebugWithAgents ? (
					<Button
						asChild
						variant="outline"
						size="sm"
						className="border-border-secondary bg-surface-primary"
					>
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
