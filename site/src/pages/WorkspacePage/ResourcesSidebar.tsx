import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import type { FC } from "react";
import type { WorkspaceResource } from "api/typesGenerated";
import {
  Sidebar,
  SidebarCaption,
  SidebarItem,
} from "components/FullPageLayout/Sidebar";
import { getResourceIconPath } from "utils/workspace";

type ResourcesSidebarProps = {
  failed: boolean;
  resources: WorkspaceResource[];
  onChange: (resource: WorkspaceResource) => void;
  isSelected: (resource: WorkspaceResource) => boolean;
};

export const ResourcesSidebar: FC<ResourcesSidebarProps> = ({
  failed,
  onChange,
  isSelected,
  resources,
}) => {
  const theme = useTheme();

  return (
    <Sidebar>
      <SidebarCaption>Resources</SidebarCaption>
      {failed && (
        <p
          css={{
            margin: 0,
            padding: "0 16px",
            fontSize: 13,
            color: theme.palette.text.secondary,
            lineHeight: "1.5",
          }}
        >
          Your workspace build failed, so the necessary resources couldn&apos;t
          be created.
        </p>
      )}
      {resources.length === 0 &&
        !failed &&
        Array.from({ length: 8 }, (_, i) => (
          <SidebarItem key={i}>
            <ResourceSidebarItemSkeleton />
          </SidebarItem>
        ))}
      {resources.map((r) => (
        <SidebarItem
          onClick={() => onChange(r)}
          isActive={isSelected(r)}
          key={r.id}
          css={styles.root}
        >
          <div
            css={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              lineHeight: 0,
              width: 16,
              height: 16,
              padding: 2,
            }}
          >
            <img
              css={{ width: "100%", height: "100%", objectFit: "contain" }}
              src={getResourceIconPath(r.type)}
              alt=""
              role="presentation"
            />
          </div>
          <div
            css={{ display: "flex", flexDirection: "column", fontWeight: 500 }}
          >
            <span>{r.name}</span>
            <span css={{ fontSize: 12, color: theme.palette.text.secondary }}>
              {r.type}
            </span>
          </div>
        </SidebarItem>
      ))}
    </Sidebar>
  );
};

export const ResourceSidebarItemSkeleton: FC = () => {
  return (
    <div css={[styles.root, { pointerEvents: "none" }]}>
      <Skeleton variant="circular" width={16} height={16} />
      <div>
        <Skeleton variant="text" width={94} height={16} />
        <Skeleton
          variant="text"
          width={60}
          height={14}
          css={{ marginTop: 2 }}
        />
      </div>
    </div>
  );
};

const styles = {
  root: {
    lineHeight: "1.5",
    display: "flex",
    alignItems: "center",
    gap: 12,
  },
} satisfies Record<string, Interpolation<Theme>>;
