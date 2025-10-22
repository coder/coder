import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import type { Workspace } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Sidebar,
	SidebarCaption,
	SidebarItem,
	SidebarLink,
} from "components/FullPageLayout/Sidebar";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Spinner } from "components/Spinner/Spinner";
import { ArrowDownIcon } from "lucide-react";
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
			<ScrollArea>
				<div className="flex flex-col gap-px">
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
							<Button
								onClick={() => buildsQuery.fetchNextPage()}
								disabled={buildsQuery.isFetchingNextPage}
								variant="outline"
								className="w-full"
							>
								<Spinner loading={buildsQuery.isFetchingNextPage}>
									<ArrowDownIcon />
								</Spinner>
								Show more builds
							</Button>
						</div>
					)}
				</div>
			</ScrollArea>
		</Sidebar>
	);
};
