import { isAxiosError } from "axios";
import dayjs from "dayjs";
import type { FC } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { useParams } from "react-router";
import { API } from "#/api/api";
import { workspaceBuildByNumber } from "#/api/queries/workspaceBuilds";
import { workspaceByOwnerAndName } from "#/api/queries/workspaces";
import { useWorkspaceBuildLogs } from "#/hooks/useWorkspaceBuildLogs";
import { linkToTemplate, useLinks } from "#/modules/navigation";
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

	// We only want to fetch the workspace if the build is not found. This is so
	// we can gather why the build is not found, specifically catching the case
	// where the workspace has been deleted. The build endpoint returns 404 today;
	// 410 Gone is the semantically correct status for a deleted workspace's build
	// and is what we'd like to migrate to in the future, so we accept both here.
	const shouldCheckDeletedWorkspace =
		isAxiosError(wsBuildQuery.error) &&
		(wsBuildQuery.error.response?.status === 404 ||
			wsBuildQuery.error.response?.status === 410);
	const workspaceQuery = useQuery({
		...workspaceByOwnerAndName(username, workspaceName),
		enabled: shouldCheckDeletedWorkspace,
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
	const getLink = useLinks();
	const workspace = workspaceQuery.data;
	const deletedWorkspaceBanner =
		workspace?.latest_build.status === "deleted"
			? {
					createWorkspaceLink: `${getLink(
						linkToTemplate(
							workspace.organization_name,
							workspace.template_name,
						),
					)}/workspace`,
					templateName:
						workspace.template_display_name || workspace.template_name,
				}
			: undefined;

	// Hold off on surfacing the build error while we follow up with the
	// workspace query, otherwise the generic error briefly flashes before the
	// deleted-workspace banner has a chance to render.
	const buildError =
		shouldCheckDeletedWorkspace && !workspaceQuery.isFetched
			? undefined
			: wsBuildQuery.error;

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
				buildError={buildError}
				deletedWorkspaceBanner={deletedWorkspaceBanner}
				builds={buildsQuery.data}
				activeBuildNumber={buildNumber}
			/>
		</>
	);
};

export default WorkspaceBuildPage;
