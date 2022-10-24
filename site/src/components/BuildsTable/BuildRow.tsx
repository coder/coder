import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { WorkspaceBuild } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { useTranslation } from "react-i18next"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import {
  displayWorkspaceBuildDuration,
  getDisplayWorkspaceBuildInitiatedBy,
} from "util/workspace"
import { BuildAvatar } from "./BuildAvatar"

export interface BuildRowProps {
  build: WorkspaceBuild
}

export const BuildRow: React.FC<BuildRowProps> = ({ build }) => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(build)

  return (
    <TableRow
      hover
      key={build.id}
      data-testid={`build-${build.id}`}
      className={styles.buildRow}
    >
      <TableCell className={styles.buildCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={styles.buildRow}
          tabIndex={0}
        >
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            className={styles.buildRowMain}
          >
            <Stack direction="row" alignItems="center">
              <BuildAvatar build={build} />
              <div>
                <Stack
                  className={styles.buildResume}
                  direction="row"
                  alignItems="center"
                  spacing={1}
                >
                  <span>
                    <strong>{initiatedBy}</strong>{" "}
                    {build.reason !== "initiator" ? "automatically " : " "}
                    <strong>
                      {t(`buildTransitionMessage.${build.transition}`)}
                    </strong>{" "}
                    the workspace
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
                </Stack>
              </div>
            </Stack>
          </Stack>
        </Stack>
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  buildRow: {
    padding: theme.spacing(2, 4),
    cursor: "pointer",

    "&:not(:last-child) td:before": {
      position: "absolute",
      top: 20,
      left: 50,
      display: "block",
      content: "''",
      height: "100%",
      width: 2,
      background: theme.palette.divider,
    },
  },

  buildCell: {
    padding: "0 !important",
    position: "relative",
    borderBottom: 0,
  },

  buildRowMain: {
    flex: 1,
  },

  buildResume: {
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
}))
