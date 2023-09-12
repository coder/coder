import { makeStyles } from "@mui/styles";
import TableCell from "@mui/material/TableCell";
import { WorkspaceBuild } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { useClickable } from "hooks/useClickable";
import { useNavigate } from "react-router-dom";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import {
  displayWorkspaceBuildDuration,
  getDisplayWorkspaceBuildInitiatedBy,
} from "utils/workspace";
import { BuildAvatar } from "components/BuildAvatar/BuildAvatar";

export interface BuildRowProps {
  build: WorkspaceBuild;
}

const transitionMessages = {
  start: "started",
  stop: "stopped",
  delete: "deleted",
};

export const BuildRow: React.FC<BuildRowProps> = ({ build }) => {
  const styles = useStyles();
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(build);
  const navigate = useNavigate();
  const clickableProps = useClickable(() =>
    navigate(`builds/${build.build_number}`),
  );

  return (
    <TimelineEntry hover data-testid={`build-${build.id}`} {...clickableProps}>
      <TableCell className={styles.buildCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={styles.buildWrapper}
        >
          <Stack
            direction="row"
            alignItems="center"
            className={styles.fullWidth}
          >
            <BuildAvatar build={build} />
            <Stack
              direction="row"
              justifyContent="space-between"
              alignItems="center"
              className={styles.fullWidth}
            >
              <Stack
                className={styles.buildSummary}
                direction="row"
                alignItems="center"
                spacing={1}
              >
                <span>
                  <strong>{initiatedBy}</strong>{" "}
                  {build.reason !== "initiator" ? "automatically" : ""}
                  <strong>{transitionMessages[build.transition]}</strong> the
                  workspace
                </span>

                <span className={styles.buildTime}>
                  {new Date(build.created_at).toLocaleTimeString()}
                </span>
              </Stack>

              <Stack direction="row" spacing={1}>
                <span className={styles.buildInfo}>
                  Reason: <strong>{build.reason}</strong>
                </span>

                <span className={styles.buildInfo}>
                  Duration:{" "}
                  <strong>{displayWorkspaceBuildDuration(build)}</strong>
                </span>

                <span className={styles.buildInfo}>
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

const useStyles = makeStyles((theme) => ({
  buildWrapper: {
    padding: theme.spacing(2, 4),
  },

  buildCell: {
    padding: "0 !important",
    position: "relative",
    borderBottom: 0,
  },

  buildSummary: {
    ...theme.typography.body1,
    fontFamily: "inherit",
  },

  buildInfo: {
    ...theme.typography.body2,
    fontSize: 12,
    fontFamily: "inherit",
    color: theme.palette.text.secondary,
    display: "block",
  },

  buildTime: {
    color: theme.palette.text.secondary,
    fontSize: 12,
  },

  buildRight: {
    width: "auto",
  },

  buildExtraInfo: {
    ...theme.typography.body2,
    fontFamily: MONOSPACE_FONT_FAMILY,
    color: theme.palette.text.secondary,
    whiteSpace: "nowrap",
  },

  fullWidth: {
    width: "100%",
  },
}));
