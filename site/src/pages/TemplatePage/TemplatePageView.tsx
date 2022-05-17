import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
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
  error?: unknown
}

export const TemplatePageView: React.FC<TemplatePageViewProps> = (props) => {
  // const styles = useStyles()
  return (
    <Stack spacing={4}>
      <Margins>Template page! {props.template?.id}</Margins>
    </Stack>
  )
}

// const useStyles = makeStyles((theme) => ({
//   actions: {
//     marginTop: theme.spacing(3),
//     marginBottom: theme.spacing(3),
//     display: "flex",
//     height: theme.spacing(6),

//     "& button": {
//       marginLeft: "auto",
//     },
//   },
//   welcome: {
//     paddingTop: theme.spacing(12),
//     paddingBottom: theme.spacing(12),
//     display: "flex",
//     flexDirection: "column",
//     alignItems: "center",
//     justifyContent: "center",
//     "& span": {
//       maxWidth: 600,
//       textAlign: "center",
//       fontSize: theme.spacing(2),
//       lineHeight: `${theme.spacing(3)}px`,
//     },
//   },
//   templateRow: {
//     "& > td": {
//       paddingTop: theme.spacing(2),
//       paddingBottom: theme.spacing(2),
//     },
//   },
//   templateAvatar: {
//     borderRadius: 2,
//     marginRight: theme.spacing(1),
//     width: 24,
//     height: 24,
//     fontSize: 16,
//   },
//   templateName: {
//     display: "flex",
//     alignItems: "center",
//   },
//   templateLink: {
//     display: "flex",
//     flexDirection: "column",
//     color: theme.palette.text.primary,
//     textDecoration: "none",
//     "&:hover": {
//       textDecoration: "underline",
//     },
//     "& span": {
//       fontSize: 12,
//       color: theme.palette.text.secondary,
//     },
//   },
// }))
