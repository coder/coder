import ReportProblemOutlinedIcon from "@mui/icons-material/ReportProblemOutlined"
import ErrorOutlineOutlinedIcon from "@mui/icons-material/ErrorOutlineOutlined"
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined"
import { Severity } from "./alertTypes"
import { ReactElement } from "react"

export const severityConstants: Record<Severity, { icon: ReactElement }> = {
  warning: {
    icon: (
      <ReportProblemOutlinedIcon
        sx={{ color: (theme) => theme.palette.warning.light, fontSize: 16 }}
      />
    ),
  },
  error: {
    icon: (
      <ErrorOutlineOutlinedIcon
        sx={{ color: (theme) => theme.palette.error.light, fontSize: 16 }}
      />
    ),
  },
  info: {
    icon: (
      <InfoOutlinedIcon
        sx={{ color: (theme) => theme.palette.info.light, fontSize: 16 }}
      />
    ),
  },
}
