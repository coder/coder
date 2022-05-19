import Avatar from "@material-ui/core/Avatar"
import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { firstLetter } from "../../util/firstLetter"

dayjs.extend(relativeTime)

export const Language = {
  createButton: "Create Template",
  emptyViewCreate: "to standardize development workspaces for your team.",
  emptyViewNoPerms: "No templates have been created! Contact your Coder administrator.",
}

export interface TemplatesPageViewProps {
  loading?: boolean
  canCreateTemplate?: boolean
  templates?: TypesGen.Template[]
  error?: unknown
}

export const TemplatesPageView: React.FC<TemplatesPageViewProps> = (props) => {
  const styles = useStyles()
  return (
    <Stack spacing={4}>
      <Margins>
        <div className={styles.actions}>
          {props.canCreateTemplate && <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>}
        </div>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Used By</TableCell>
              <TableCell>Last Updated</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {!props.loading && !props.templates?.length && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div className={styles.welcome}>
                    {props.canCreateTemplate ? (
                      <span>
                        <Link component={RouterLink} to="/templates/new">
                          Create a template
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
            {props.templates?.map((template) => {
              return (
                <TableRow key={template.id} className={styles.templateRow}>
                  <TableCell>
                    <div className={styles.templateName}>
                      <Avatar variant="square" className={styles.templateAvatar}>
                        {firstLetter(template.name)}
                      </Avatar>
                      <Link component={RouterLink} to={`/templates/${template.id}`} className={styles.templateLink}>
                        <b>{template.name}</b>
                        <span>{template.description}</span>
                      </Link>
                    </div>
                  </TableCell>
                  <TableCell>
                    {template.workspace_owner_count} developer{template.workspace_owner_count !== 1 && "s"}
                  </TableCell>
                  <TableCell data-chromatic="ignore">{dayjs().to(dayjs(template.updated_at))}</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  actions: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
    display: "flex",
    height: theme.spacing(6),

    "& button": {
      marginLeft: "auto",
    },
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
  templateRow: {
    "& > td": {
      paddingTop: theme.spacing(2),
      paddingBottom: theme.spacing(2),
    },
  },
  templateAvatar: {
    borderRadius: 2,
    marginRight: theme.spacing(1),
    width: 24,
    height: 24,
    fontSize: 16,
  },
  templateName: {
    display: "flex",
    alignItems: "center",
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
