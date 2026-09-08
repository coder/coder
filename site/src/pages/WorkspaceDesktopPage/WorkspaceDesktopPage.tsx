import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { workspaceByOwnerAndName } from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { WorkspaceDesktop } from "#/modules/desktop/WorkspaceDesktop";
import { pageTitle } from "#/utils/page";
import { getMatchingAgentOrFirst } from "#/utils/workspace";

/**
 * WorkspaceDesktopPage shows the built-in virtual desktop of a workspace
 * agent full screen, mirroring the web terminal page. The route is
 * /@:username/:workspace[.:agent]/desktop.
 */
const WorkspaceDesktopPage: FC = () => {
	const params = useParams() as { username: string; workspace: string };
	const username = params.username.replace("@", "");

	// The workspace name is in the format:
	// <workspace name>[.<agent name>]
	const workspaceNameParts = params.workspace?.split(".");
	const workspace = useQuery(
		workspaceByOwnerAndName(username, workspaceNameParts?.[0]),
	);
	const workspaceAgent = workspace.data
		? getMatchingAgentOrFirst(workspace.data, workspaceNameParts?.[1])
		: undefined;

	return (
		<div className="h-screen w-screen bg-surface-secondary">
			{workspace.data && (
				<title>
					{pageTitle(
						"Desktop",
						`${workspace.data.owner_name}/${workspace.data.name}`,
					)}
				</title>
			)}
			{workspace.isLoading && <Loader fullscreen />}
			{workspace.error && (
				<div className="p-6">
					<ErrorAlert error={workspace.error} />
				</div>
			)}
			{workspace.data && !workspaceAgent && (
				<div className="p-6">
					<ErrorAlert error="This workspace has no agent to connect to." />
				</div>
			)}
			{workspaceAgent && <WorkspaceDesktop agentId={workspaceAgent.id} />}
		</div>
	);
};

export default WorkspaceDesktopPage;
