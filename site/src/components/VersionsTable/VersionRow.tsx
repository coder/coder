import { makeStyles } from "@material-ui/core/styles"
import TableCell from "@material-ui/core/TableCell"
import TableRow from "@material-ui/core/TableRow"
import { TemplateVersion } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import { useTranslation } from "react-i18next"

export interface VersionRowProps {
  version: TemplateVersion
}

export const VersionRow: React.FC<VersionRowProps> = ({ version }) => {
  const styles = useStyles()
  const { t } = useTranslation("templatePage")

  return (
    <TableRow
      className={styles.versionRow}
      data-testid={`version-${version.id}`}
    >
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
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  versionRow: {
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
