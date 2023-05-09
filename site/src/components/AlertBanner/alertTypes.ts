import { ApiError } from "api/errors"
import { ReactElement } from "react"

export type Severity = "warning" | "error" | "info"

export interface AlertBannerProps {
  severity: Severity
  text?: JSX.Element
  error?: ApiError | Error | unknown
  actions?: ReactElement[]
  dismissible?: boolean
  onDismiss?: () => void
  retry?: () => void
}
