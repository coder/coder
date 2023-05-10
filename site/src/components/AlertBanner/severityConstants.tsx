import ReportProblemOutlinedIcon from "@mui/icons-material/ReportProblemOutlined"
import ErrorOutlineOutlinedIcon from "@mui/icons-material/ErrorOutlineOutlined"
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined"
import { colors } from "theme/colors"
import { Severity } from "./alertTypes"
import { ReactElement } from "react"

export const severityConstants: Record<
  Severity,
  { color: string; icon: ReactElement }
> = {
  warning: {
    color: colors.orange[7],
    icon: (
      <ReportProblemOutlinedIcon
        style={{ color: colors.orange[7], fontSize: 16 }}
      />
    ),
  },
  error: {
    color: colors.red[7],
    icon: (
      <ErrorOutlineOutlinedIcon
        style={{ color: colors.red[7], fontSize: 16 }}
      />
    ),
  },
  info: {
    color: colors.blue[7],
    icon: <InfoOutlinedIcon style={{ color: colors.blue[7], fontSize: 16 }} />,
  },
}
