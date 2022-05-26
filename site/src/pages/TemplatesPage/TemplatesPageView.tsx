import Avatar from "@material-ui/core/Avatar"
import Box from "@material-ui/core/Box"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import { firstLetter } from "../../util/firstLetter"

dayjs.extend(relativeTime)

export const Language = {
  developerCount: (ownerCount: number): string => {
    return `${ownerCount} developer${ownerCount !== 1 ? "s" : ""}`
  },
  nameLabel: "Name",
  usedByLabel: "Used by",
  lastUpdatedLabel: "Last updated",
  emptyViewCreateCTA: "Create a template",
  emptyViewCreate: "to standardize development workspaces for your team.",
  emptyViewNoPerms: "No templates have been created! Contact your Coder administrator.",
}

export interface TemplatesPageViewProps {
  loading?: boolean
  canCreateTemplate?: boolean
  templates?: TypesGen.Template[]
}

export const TemplatesPageView: React.FC<TemplatesPageViewProps> = (props) => {
  const styles = useStyles()
  return (
    <Stack spacing={4} className={styles.root}>
      <Margins>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>{Language.nameLabel}</TableCell>
              <TableCell>{Language.usedByLabel}</TableCell>
              <TableCell>{Language.lastUpdatedLabel}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {props.loading && <TableLoader />}
            {!props.loading && !props.templates?.length && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div className={styles.welcome}>
                    {props.canCreateTemplate ? (
                      <span>
                        <Link component={RouterLink} to="/templates/new">
                          {Language.emptyViewCreateCTA}
                        </Link>
                        &nbsp;{Language.emptyViewCreate}
                      </span>
                    ) : (
                      <span>{Language.emptyViewNoPerms}</span>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            )}
            {props.templates?.map((template) => (
              <TableRow key={template.id}>
                <TableCell>
                  <Box alignItems="center" display="flex">
                    <Avatar variant="square" className={styles.templateAvatar}>
                      {firstLetter(template.name)}
                    </Avatar>
                    <Link component={RouterLink} to={`/templates/${template.name}`} className={styles.templateLink}>
                      <b>{template.name}</b>
                      <span>{template.description}</span>
                    </Link>
                  </Box>
                </TableCell>

                <TableCell>{Language.developerCount(template.workspace_owner_count)}</TableCell>

                <TableCell data-chromatic="ignore">{dayjs().to(dayjs(template.updated_at))}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    marginTop: theme.spacing(3),
  },
  welcome: {
    paddingTop: theme.spacing(12),
    paddingBottom: theme.spacing(12),
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",
    "& span": {
      maxWidth: 600,
      textAlign: "center",
      fontSize: theme.spacing(2),
      lineHeight: `${theme.spacing(3)}px`,
    },
  },
  templateAvatar: {
    borderRadius: 2,
    marginRight: theme.spacing(1),
    width: 24,
    height: 24,
    fontSize: 16,
  },
  templateLink: {
    display: "flex",
    flexDirection: "column",
    color: theme.palette.text.primary,
    textDecoration: "none",
    "&:hover": {
      textDecoration: "underline",
    },
    "& span": {
      fontSize: 12,
      color: theme.palette.text.secondary,
    },
  },
}))
