import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import ReactMarkdown from "react-markdown"
import * as TypesGen from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"

dayjs.extend(relativeTime)

export const Language = {
  createButton: "Create Template",
  emptyViewCreate: "to standardize development workspaces for your team.",
  emptyViewNoPerms: "No template have been created! Contact your Coder administrator.",
}

export interface TemplatePageViewProps {
  loading?: boolean
  template?: TypesGen.Template
  templateVersion?: TypesGen.TemplateVersion
  error?: unknown
}

export const TemplatePageView: React.FC<TemplatePageViewProps> = (props) => {
  const styles = useStyles()
  return (
    <Stack spacing={4}>
      <Margins>
        Template page! {props.template?.name}
        {props.templateVersion?.readme && (
          <Paper className={styles.readme}>
            <ReactMarkdown>{props.templateVersion.readme}</ReactMarkdown>
          </Paper>
        )}
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  readme: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),

    "& img": {
      // Prevents overflow!
      maxWidth: "100%",
    },
  },
}))
