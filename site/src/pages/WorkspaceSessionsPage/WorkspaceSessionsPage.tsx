import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { paginatedWorkspaceSessions } from "api/queries/workspaceSessions";
import { isNonInitialPage } from "components/PaginationWidget/utils";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { WorkspaceSessionsPageView } from "./WorkspaceSessionsPageView";

const WorkspaceSessionsPage: FC = () => {
	const params = useParams() as { username: string; workspace: string };
	const username = params.username.replace("@", "");
	const workspaceName = params.workspace;
	const [searchParams] = useSearchParams();

	const workspaceQuery = useQuery(
		workspaceByOwnerAndName(username, workspaceName),
	);
	const workspace = workspaceQuery.data;

	const sessionsQuery = usePaginatedQuery(
		paginatedWorkspaceSessions(workspace?.id),
	);

	return (
		<>
			<title>{pageTitle("Session History")}</title>
			<WorkspaceSessionsPageView
				workspace={workspace}
				sessions={sessionsQuery.data?.sessions}
				sessionsQuery={sessionsQuery}
				isNonInitialPage={isNonInitialPage(searchParams)}
				error={sessionsQuery.error}
			/>
		</>
	);
};

export default WorkspaceSessionsPage;
