import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import TableCell from "@mui/material/TableCell";
import type { FC } from "react";
import { useNavigate } from "react-router-dom";
import type { WorkspaceBuild } from "api/typesGenerated";
import { BuildAvatar } from "components/BuildAvatar/BuildAvatar";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { useClickable } from "hooks/useClickable";
import {
  displayWorkspaceBuildDuration,
  getDisplayWorkspaceBuildInitiatedBy,
} from "utils/workspace";

export interface BuildRowProps {
  build: WorkspaceBuild;
}

const transitionMessages = {
  start: "started",
  stop: "stopped",
  delete: "deleted",
};

export const BuildRow: FC<BuildRowProps> = ({ build }) => {
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(build);
  const navigate = useNavigate();
  const clickableProps = useClickable<HTMLTableRowElement>(() =>
    navigate(`builds/${build.build_number}`),
  );

  return (
    <TimelineEntry hover data-testid={`build-${build.id}`} {...clickableProps}>
      <TableCell css={styles.buildCell}>
        <Stack direction="row" alignItems="center" css={styles.buildWrapper}>
          <Stack direction="row" alignItems="center" css={styles.fullWidth}>
            <BuildAvatar build={build} />
            <Stack
              direction="row"
              justifyContent="space-between"
              alignItems="center"
              css={styles.fullWidth}
            >
              <Stack
                css={styles.buildSummary}
                direction="row"
                alignItems="center"
                spacing={1}
              >
                <span>
                  <strong>{initiatedBy}</strong>{" "}
                  {build.reason !== "initiator" ? "automatically " : ""}
                  <strong>{transitionMessages[build.transition]}</strong> the
                  workspace
                </span>

                <span css={styles.buildTime}>
                  {new Date(build.created_at).toLocaleTimeString()}
                </span>
              </Stack>

              <Stack
                direction="row"
                spacing={1}
                css={{ "& strong": { fontWeight: 600 } }}
              >
                <span css={styles.buildInfo}>
                  Reason: <strong>{build.reason}</strong>
                </span>

                <span css={styles.buildInfo}>
                  Duration:{" "}
                  <strong>{displayWorkspaceBuildDuration(build)}</strong>
                </span>

                <span css={styles.buildInfo}>
                  Version: <strong>{build.template_version_name}</strong>
                </span>
              </Stack>
            </Stack>
          </Stack>
        </Stack>
      </TableCell>
    </TimelineEntry>
  );
};

const styles = {
  buildWrapper: {
    padding: "16px 32px",
  },

  buildCell: {
    padding: "0 !important",
    position: "relative",
    borderBottom: 0,
  },

  buildSummary: (theme) => ({
    ...(theme.typography.body1 as CSSObject),
    fontFamily: "inherit",
    "& strong": {
      fontWeight: 600,
    },
  }),

  buildInfo: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    fontSize: 12,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  }),

  buildTime: (theme) => ({
    color: theme.palette.text.secondary,
    fontSize: 12,
  }),

  fullWidth: {
    width: "100%",
  },
} satisfies Record<string, Interpolation<Theme>>;
