import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import { TemplateVersion } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { TimelineEntry } from "components/Timeline/TimelineEntry"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { useClickable } from "hooks/useClickable"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"

export interface VersionRowProps {
  version: TemplateVersion
}

export const VersionRow: React.FC<VersionRowProps> = ({ version }) => {
  const styles = useStyles()
  const { t } = useTranslation("templatePage")
  const navigate = useNavigate()
  const clickableProps = useClickable(() => {
    navigate(`versions/${version.name}`)
  })

  return (
    <TimelineEntry data-testid={`version-${version.id}`} {...clickableProps}>
      <TableCell className={styles.versionCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={styles.versionWrapper}
        >
          <Stack direction="row" alignItems="center">
            <UserAvatar
              username={version.created_by.username}
              avatarURL={version.created_by.avatar_url}
            />
            <Stack
              className={styles.versionSummary}
              direction="row"
              alignItems="center"
              spacing={1}
            >
              <span>
                <strong>{version.created_by.username}</strong>{" "}
                {t("createdVersion")} <strong>{version.name}</strong>
              </span>

              <span className={styles.versionTime}>
                {new Date(version.created_at).toLocaleTimeString()}
              </span>
            </Stack>
          </Stack>
        </Stack>
      </TableCell>
    </TimelineEntry>
  )
}

const useStyles = makeStyles((theme) => ({
  versionWrapper: {
    padding: theme.spacing(2, 4),
  },

  versionCell: {
    padding: "0 !important",
    position: "relative",
    borderBottom: 0,
  },

  versionSummary: {
    ...theme.typography.body1,
    fontFamily: "inherit",
  },

  versionTime: {
    color: theme.palette.text.secondary,
    fontSize: 12,
  },
}))
