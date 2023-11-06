import { type FC, type PropsWithChildren, useState } from "react";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { type CSSObject, type Interpolation, type Theme } from "@emotion/react";
import { Children } from "react";
import type { WorkspaceAgent, WorkspaceResource } from "api/typesGenerated";
import { DropdownArrow } from "../DropdownArrow/DropdownArrow";
import { CopyableValue } from "../CopyableValue/CopyableValue";
import { MemoizedInlineMarkdown } from "../Markdown/Markdown";
import { Stack } from "../Stack/Stack";
import { ResourceAvatar } from "./ResourceAvatar";
import { SensitiveValue } from "./SensitiveValue";

const styles = {
  resourceCard: (theme) => ({
    background: theme.palette.background.paper,
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,

    "&:not(:first-of-type)": {
      borderTop: 0,
      borderTopLeftRadius: 0,
      borderTopRightRadius: 0,
    },

    "&:not(:last-child)": {
      borderBottomLeftRadius: 0,
      borderBottomRightRadius: 0,
    },
  }),

  resourceCardProfile: {
    flexShrink: 0,
    width: "fit-content",
  },

  resourceCardHeader: (theme) => ({
    padding: "24px 32px",
    borderBottom: `1px solid ${theme.palette.divider}`,

    "&:last-child": {
      borderBottom: 0,
    },

    [theme.breakpoints.down("md")]: {
      width: "100%",
      overflow: "scroll",
    },
  }),

  metadata: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    lineHeight: "120%",
  }),

  metadataLabel: (theme) => ({
    fontSize: 12,
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
  }),

  metadataValue: (theme) => ({
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    ...(theme.typography.body1 as CSSObject),
  }),
} satisfies Record<string, Interpolation<Theme>>;

export interface ResourceCardProps {
  resource: WorkspaceResource;
  agentRow: (agent: WorkspaceAgent) => JSX.Element;
}

const p = ({ children }: PropsWithChildren) => {
  const childrens = Children.toArray(children);
  if (childrens.every((child) => typeof child === "string")) {
    return <CopyableValue value={childrens.join("")}>{children}</CopyableValue>;
  }
  return <>{children}</>;
};

export const ResourceCard: FC<ResourceCardProps> = ({ resource, agentRow }) => {
  const [shouldDisplayAllMetadata, setShouldDisplayAllMetadata] =
    useState(false);
  const metadataToDisplay = resource.metadata ?? [];
  const visibleMetadata = shouldDisplayAllMetadata
    ? metadataToDisplay
    : metadataToDisplay.slice(0, 4);

  // Add one to `metadataLength` if the resource has a cost, and hide one
  // additional metadata item, because cost is displayed in the same grid.
  let metadataLength = resource.metadata?.length ?? 0;
  if (resource.daily_cost > 0) {
    metadataLength += 1;
    visibleMetadata.pop();
  }
  const gridWidth = metadataLength === 1 ? 1 : 4;

  return (
    <div key={resource.id} css={styles.resourceCard} className="resource-card">
      <Stack
        direction="row"
        alignItems="flex-start"
        css={styles.resourceCardHeader}
        spacing={10}
      >
        <Stack
          direction="row"
          alignItems="center"
          css={styles.resourceCardProfile}
        >
          <div>
            <ResourceAvatar resource={resource} />
          </div>
          <div css={styles.metadata}>
            <div css={styles.metadataLabel}>{resource.type}</div>
            <div css={styles.metadataValue}>{resource.name}</div>
          </div>
        </Stack>

        <div
          css={{
            flexGrow: 2,
            display: "grid",
            gridTemplateColumns: `repeat(${gridWidth}, minmax(0, 1fr))`,
            gap: 40,
            rowGap: 24,
          }}
        >
          {resource.daily_cost > 0 && (
            <div css={styles.metadata}>
              <div css={styles.metadataLabel}>
                <b>cost</b>
              </div>
              <div css={styles.metadataValue}>{resource.daily_cost}</div>
            </div>
          )}
          {visibleMetadata.map((meta) => {
            return (
              <div css={styles.metadata} key={meta.key}>
                <div css={styles.metadataLabel}>{meta.key}</div>
                <div css={styles.metadataValue}>
                  {meta.sensitive ? (
                    <SensitiveValue value={meta.value} />
                  ) : (
                    <MemoizedInlineMarkdown components={{ p }}>
                      {meta.value}
                    </MemoizedInlineMarkdown>
                  )}
                </div>
              </div>
            );
          })}
        </div>
        {metadataLength > 4 && (
          <Tooltip
            title={
              shouldDisplayAllMetadata ? "Hide metadata" : "Show all metadata"
            }
          >
            <IconButton
              onClick={() => {
                setShouldDisplayAllMetadata((value) => !value);
              }}
              size="large"
            >
              <DropdownArrow margin={false} close={shouldDisplayAllMetadata} />
            </IconButton>
          </Tooltip>
        )}
      </Stack>

      {resource.agents && resource.agents.length > 0 && (
        <div>{resource.agents.map(agentRow)}</div>
      )}
    </div>
  );
};
