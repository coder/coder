import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import type { WorkspaceAgent } from "api/typesGenerated";
import { Stack } from "../Stack/Stack";
import { AppPreviewLink } from "./AppLink/AppPreviewLink";

interface AgentRowPreviewStyles {
  // Helpful when there are more than one row so the values are aligned
  // When it is only one row, it is better to have than "flex" and not hard aligned
  alignValues?: boolean;
}
export interface AgentRowPreviewProps extends AgentRowPreviewStyles {
  agent: WorkspaceAgent;
}

export const AgentRowPreview: FC<AgentRowPreviewProps> = ({
  agent,
  alignValues,
}) => {
  return (
    <Stack
      key={agent.id}
      direction="row"
      alignItems="center"
      justifyContent="space-between"
      css={styles.agentRow}
    >
      <Stack direction="row" alignItems="baseline">
        <div css={styles.agentStatusWrapper}>
          <div css={styles.agentStatusPreview}></div>
        </div>
        <Stack
          alignItems="baseline"
          direction="row"
          spacing={4}
          css={styles.agentData}
        >
          <Stack
            direction="row"
            alignItems="baseline"
            spacing={1}
            css={[
              styles.noShrink,
              styles.agentDataItem,
              (theme) => ({
                [theme.breakpoints.up("sm")]: {
                  minWidth: alignValues ? 240 : undefined,
                },
              }),
            ]}
          >
            <span>Agent:</span>
            <span css={styles.agentDataValue}>{agent.name}</span>
          </Stack>

          <Stack
            direction="row"
            alignItems="baseline"
            spacing={1}
            css={[
              styles.noShrink,
              styles.agentDataItem,
              (theme) => ({
                [theme.breakpoints.up("sm")]: {
                  minWidth: alignValues ? 100 : undefined,
                },
              }),
            ]}
          >
            <span>OS:</span>
            <span css={[styles.agentDataValue, styles.agentOS]}>
              {agent.operating_system}
            </span>
          </Stack>

          <Stack
            direction="row"
            alignItems="center"
            spacing={1}
            css={styles.agentDataItem}
          >
            <span>Apps:</span>
            <Stack
              direction="row"
              alignItems="center"
              spacing={0.5}
              wrap="wrap"
            >
              {agent.apps.map((app) => (
                <AppPreviewLink key={app.slug} app={app} />
              ))}
              {agent.apps.length === 0 && (
                <span css={styles.agentDataValue}>None</span>
              )}
            </Stack>
          </Stack>
        </Stack>
      </Stack>
    </Stack>
  );
};

const styles = {
  agentRow: (theme) => ({
    padding: "16px 32px",
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,
    position: "relative",

    "&:not(:last-child)": {
      paddingBottom: 0,
    },

    "&:after": {
      content: "''",
      height: "100%",
      width: 2,
      backgroundColor: theme.palette.divider,
      position: "absolute",
      top: 0,
      left: 49,
    },
  }),

  agentStatusWrapper: {
    width: 36,
    display: "flex",
    justifyContent: "center",
    flexShrink: 0,
  },

  agentStatusPreview: (theme) => ({
    width: 10,
    height: 10,
    border: `2px solid ${theme.palette.text.secondary}`,
    borderRadius: "100%",
    position: "relative",
    zIndex: 1,
    background: theme.palette.background.paper,
  }),

  agentName: {
    fontWeight: 600,
  },

  agentOS: {
    textTransform: "capitalize",
    fontSize: 14,
  },

  agentData: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,

    [theme.breakpoints.down("md")]: {
      gap: 16,
      flexWrap: "wrap",
    },
  }),

  agentDataValue: (theme) => ({
    color: theme.palette.text.primary,
  }),

  noShrink: {
    flexShrink: 0,
  },

  agentDataItem: (theme) => ({
    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
      alignItems: "flex-start",
      gap: 8,
      width: "fit-content",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
