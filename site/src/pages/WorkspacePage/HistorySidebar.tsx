import ArrowDownwardOutlined from "@mui/icons-material/ArrowDownwardOutlined";
import LoadingButton from "@mui/lab/LoadingButton";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import { Workspace } from "api/typesGenerated";
import {
  Sidebar,
  SidebarCaption,
  SidebarItem,
  SidebarLink,
} from "components/FullPageLayout/Sidebar";
import {
  WorkspaceBuildData,
  WorkspaceBuildDataSkeleton,
} from "components/WorkspaceBuild/WorkspaceBuildData";
import { useInfiniteQuery } from "react-query";

export const HistorySidebar = ({ workspace }: { workspace: Workspace }) => {
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
