import { useTheme } from "@mui/material/styles";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import { Workspace } from "api/typesGenerated";
import {
  Sidebar,
  SidebarCaption,
  SidebarItem,
  SidebarLink,
} from "components/FullPageLayout/Sidebar";
import { Timeline } from "components/Timeline/Timeline";
import {
  WorkspaceBuildData,
  WorkspaceBuildDataSkeleton,
} from "components/WorkspaceBuild/WorkspaceBuildData";
import { useInfiniteQuery } from "react-query";

export const HistorySidebar = ({ workspace }: { workspace: Workspace }) => {
  const theme = useTheme();
  const buildsQuery = useInfiniteQuery({
    ...infiniteWorkspaceBuilds(workspace?.id ?? ""),
    enabled: workspace !== undefined,
  });
  const builds = buildsQuery.data?.pages.flat();

  return (
    <Sidebar>
      <SidebarCaption>History</SidebarCaption>
      {builds ? (
        <Timeline
          items={builds}
          getDate={(build) => new Date(build.created_at)}
          dateRow={({ displayDate }) => (
            <div
              css={{
                fontSize: 12,
                color: theme.palette.text.secondary,
                padding: "0 16px 4px",

                "&:not(:first-of-type)": {
                  marginTop: "8px",
                },

                "&::first-letter": {
                  textTransform: "uppercase",
                },
              }}
            >
              {displayDate}
            </div>
          )}
          row={(build) => (
            <SidebarLink
              key={build.id}
              to={`/@${build.workspace_owner_name}/${build.workspace_name}/builds/${build.build_number}`}
            >
              <WorkspaceBuildData build={build} />
            </SidebarLink>
          )}
        />
      ) : (
        // builds.map((build) => (
        //       <SidebarLink
        //         key={build.id}
        //         to={`/@${build.workspace_owner_name}/${build.workspace_name}/builds/${build.build_number}`}
        //       >
        //         <WorkspaceBuildData build={build} />
        //       </SidebarLink>

        //     ))
        Array.from({ length: 15 }, (_, i) => (
          <SidebarItem key={i}>
            <WorkspaceBuildDataSkeleton />
          </SidebarItem>
        ))
      )}
    </Sidebar>
  );
};
