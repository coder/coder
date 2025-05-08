import LoadingButton from "@mui/lab/LoadingButton";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import type { Workspace } from "api/typesGenerated";
import {
	Sidebar,
	SidebarCaption,
	SidebarItem,
	SidebarLink,
} from "components/FullPageLayout/Sidebar";
import { ArrowDownwardOutlined } from "lucide-react";
import {
	WorkspaceBuildData,
	WorkspaceBuildDataSkeleton,
} from "modules/workspaces/WorkspaceBuildData/WorkspaceBuildData";
import type { FC } from "react";
import { useInfiniteQuery } from "react-query";

interface HistorySidebarProps {
	workspace: Workspace;
}

export const HistorySidebar: FC<HistorySidebarProps> = ({ workspace }) => {
	const buildsQuery = useInfiniteQuery({
		...infiniteWorkspaceBuilds(workspace?.id ?? ""),
		enabled: workspace !== undefined,
	});
	const builds = buildsQuery.data?.pages.flat();

	return (
		<Sidebar>
			<SidebarCaption>History</SidebarCaption>
			{builds
				? builds.map((build) => (
						<SidebarLink
							target="_blank"
							key={build.id}
							to={`/@${build.workspace_owner_name}/${build.workspace_name}/builds/${build.build_number}`}
						>
							<WorkspaceBuildData build={build} />
						</SidebarLink>
					))
				: Array.from({ length: 15 }, (_, i) => (
						<SidebarItem key={i}>
							<WorkspaceBuildDataSkeleton />
						</SidebarItem>
					))}
			{buildsQuery.hasNextPage && (
				<div css={{ padding: 16 }}>
					<LoadingButton
						fullWidth
						onClick={() => buildsQuery.fetchNextPage()}
						loading={buildsQuery.isFetchingNextPage}
						loadingPosition="start"
						variant="outlined"
						color="neutral"
						startIcon={<ArrowDownwardOutlined />}
						css={{
							display: "inline-flex",
							borderRadius: "9999px",
							fontSize: 13,
						}}
					>
						Show more builds
					</LoadingButton>
				</div>
			)}
		</Sidebar>
	);
};
