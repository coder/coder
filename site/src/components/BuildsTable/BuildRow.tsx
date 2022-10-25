import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { WorkspaceBuild } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { useClickable } from "hooks/useClickable"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
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
  const navigate = useNavigate()
  const clickableProps = useClickable(() =>
    navigate(`builds/${build.build_number}`),
  )

  return (
    <TableRow
      hover
      data-testid={`build-${build.id}`}
      className={styles.buildRow}
      {...clickableProps}
    >
      <TableCell className={styles.buildCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={styles.buildWrapper}
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
                  {build.reason !== "initiator"
                    ? t("buildMessage.automatically")
                    : ""}
                  <strong>{t(`buildMessage.${build.transition}`)}</strong>{" "}
                  {t("buildMessage.theWorkspace")}
                </span>

                <span className={styles.buildTime}>
                  {new Date(build.created_at).toLocaleTimeString()}
                </span>
              </Stack>

              <Stack direction="row" spacing={1}>
                <span className={styles.buildInfo}>
                  {t("buildData.reason")}: <strong>{build.reason}</strong>
                </span>

                <span className={styles.buildInfo}>
                  {t("buildData.duration")}:{" "}
                  <strong>{displayWorkspaceBuildDuration(build)}</strong>
                </span>
              </Stack>
            </div>
          </Stack>
        </Stack>
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  buildRow: {
    cursor: "pointer",

    "&:focus": {
      outlineStyle: "solid",
      outlineOffset: -1,
      outlineWidth: 2,
      outlineColor: theme.palette.secondary.dark,
    },

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

  buildWrapper: {
    padding: theme.spacing(2, 4),
  },

  buildCell: {
    padding: "0 !important",
    position: "relative",
    borderBottom: 0,
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
