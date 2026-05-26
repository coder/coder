import { isAxiosError } from "axios";
import dayjs from "dayjs";
import type { FC } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { useParams } from "react-router";
import { API } from "#/api/api";
import { workspaceBuildByNumber } from "#/api/queries/workspaceBuilds";
import { workspaceByOwnerAndName } from "#/api/queries/workspaces";
import { useWorkspaceBuildLogs } from "#/hooks/useWorkspaceBuildLogs";
import { pageTitle } from "#/utils/page";
import { WorkspaceBuildPageView } from "./WorkspaceBuildPageView";

const WorkspaceBuildPage: FC = () => {
	const params = useParams() as {
		username: string;
		workspace: string;
		buildNumber: string;
	};
	const workspaceName = params.workspace;
	const buildNumber = Number(params.buildNumber);
	const username = params.username.replace("@", "");

	const wsBuildQuery = useQuery({
		...workspaceBuildByNumber(username, workspaceName, buildNumber),
		placeholderData: keepPreviousData,
	});
	const workspaceQuery = useQuery({
		...workspaceByOwnerAndName(username, workspaceName),
		// We don't want to fetch the workspace if the build is not found.
		// This is so we can gather why the build is not found, specifically
		// catching the case where the workspace has been deleted.
		enabled:
			isAxiosError(wsBuildQuery.error) &&
			(wsBuildQuery.error.response?.status === 404 ||
				wsBuildQuery.error.response?.status === 410),
	});
	const build = wsBuildQuery.data;
	const buildsQuery = useQuery({
		queryKey: ["builds", username, build?.workspace_id],
		queryFn: () => {
			return API.getWorkspaceBuilds(build?.workspace_id ?? "", {
				since: dayjs().add(-30, "day").toISOString(),
			});
		},
		enabled: Boolean(build),
	});
	const logs = useWorkspaceBuildLogs(build?.id);

	return (
		<>
			{build && (
				<title>
					{pageTitle(`Build #${build.build_number} · ${build.workspace_name}`)}
				</title>
			)}

			<WorkspaceBuildPageView
				logs={logs}
				build={build}
				buildError={wsBuildQuery.error}
				workspace={workspaceQuery.data}
				builds={buildsQuery.data}
				activeBuildNumber={buildNumber}
			/>
		</>
	);
};

export default WorkspaceBuildPage;
