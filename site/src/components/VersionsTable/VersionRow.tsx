import Button from "@mui/material/Button"
import { makeStyles } from "@mui/styles"
import TableCell from "@mui/material/TableCell"
import { TemplateVersion } from "api/typesGenerated"
import { Pill } from "components/Pill/Pill"
import { Stack } from "components/Stack/Stack"
import { TimelineEntry } from "components/Timeline/TimelineEntry"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { useClickableTableRow } from "hooks/useClickableTableRow"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import { colors } from "theme/colors"
import { combineClasses } from "utils/combineClasses"

export interface VersionRowProps {
  version: TemplateVersion
  isActive: boolean
  onPromoteClick?: (templateVersionId: string) => void
}

export const VersionRow: React.FC<VersionRowProps> = ({
  version,
  isActive,
  onPromoteClick,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("templatePage")
  const navigate = useNavigate()
  const clickableProps = useClickableTableRow(() => {
    navigate(version.name)
  })

  return (
    <TimelineEntry
      data-testid={`version-${version.id}`}
      {...clickableProps}
      className={combineClasses({
        [clickableProps.className]: true,
        [styles.row]: true,
        [styles.active]: isActive,
      })}
    >
      <TableCell className={styles.versionCell}>
        <Stack
          direction="row"
          alignItems="center"
          className={styles.versionWrapper}
          justifyContent="space-between"
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
          {isActive ? (
            <Pill text="Active version" type="success" />
          ) : (
            onPromoteClick && (
              <Button
                size="small"
                className={styles.promoteButton}
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  onPromoteClick(version.id)
                }}
              >
                Promote version
              </Button>
            )
          )}
        </Stack>
      </TableCell>
    </TimelineEntry>
  )
}

const useStyles = makeStyles((theme) => ({
  row: {
    "&:hover $promoteButton": {
      color: theme.palette.text.primary,
      borderColor: colors.gray[11],
      "&:hover": {
        borderColor: theme.palette.text.primary,
      },
    },
  },

  promoteButton: {
    color: theme.palette.text.secondary,
    transition: "none",
  },

  versionWrapper: {
    padding: theme.spacing(2, 4),
  },

  active: {
    backgroundColor: theme.palette.background.paperLight,
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
