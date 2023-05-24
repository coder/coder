import { useState, FC, ReactNode, PropsWithChildren } from "react"
import Collapse from "@mui/material/Collapse"
// eslint-disable-next-line no-restricted-imports -- It is the base component
import MuiAlert, { AlertProps as MuiAlertProps } from "@mui/material/Alert"
import Button from "@mui/material/Button"

export interface AlertProps extends PropsWithChildren {
  severity: MuiAlertProps["severity"]
  actions?: ReactNode[]
  dismissible?: boolean
  onRetry?: () => void
  onDismiss?: () => void
}

export const Alert: FC<AlertProps> = ({
  children,
  actions = [],
  onRetry,
  dismissible,
  severity,
  onDismiss,
}) => {
  const [open, setOpen] = useState(true)

  return (
    <Collapse in={open}>
      <MuiAlert
        severity={severity}
        action={
          <>
            {/* CTAs passed in by the consumer */}
            {actions.length > 0 &&
              actions.map((action) => <div key={String(action)}>{action}</div>)}

            {/* retry CTA */}
            {onRetry && (
              <Button variant="text" size="small" onClick={onRetry}>
                Retry
              </Button>
            )}

            {/* close CTA */}
            {dismissible && (
              <Button
                variant="text"
                size="small"
                onClick={() => {
                  setOpen(false)
                  onDismiss && onDismiss()
                }}
                data-testid="dismiss-banner-btn"
              >
                Dismiss
              </Button>
            )}
          </>
        }
      >
        {children}
      </MuiAlert>
    </Collapse>
  )
}
