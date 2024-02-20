import { useTheme } from "@emotion/react";
import { Workspace } from "api/typesGenerated";
import { SidebarLink, SidebarCaption } from "components/FullPageLayout/Sidebar";

export const ResourcesSidebarContent = ({
  workspace,
}: {
  workspace: Workspace;
}) => {
  const theme = useTheme();

  return (
    <>
      <SidebarCaption>Resources</SidebarCaption>
      {workspace.latest_build.resources.map((r) => (
        <SidebarLink
          key={r.id}
          to={{ search: `r=${r.id}` }}
          css={{ display: "flex", flexDirection: "column", lineHeight: 1.6 }}
        >
          <span css={{ fontWeight: 500 }}>{r.name}</span>
          <span css={{ fontSize: 13, color: theme.palette.text.secondary }}>
            {r.type}
          </span>
        </SidebarLink>
      ))}
    </>
  );
};
