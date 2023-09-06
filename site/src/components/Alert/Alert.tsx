import { useState, FC, ReactNode } from "react";
import Collapse from "@mui/material/Collapse";
// eslint-disable-next-line no-restricted-imports -- It is the base component
import MuiAlert, { AlertProps as MuiAlertProps } from "@mui/material/Alert";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";

export type AlertProps = MuiAlertProps & {
  actions?: ReactNode;
  dismissible?: boolean;
  onRetry?: () => void;
  onDismiss?: () => void;
};

export const Alert: FC<AlertProps> = ({
  children,
  actions,
  onRetry,
  dismissible,
  severity,
  onDismiss,
  ...alertProps
}) => {
  const [open, setOpen] = useState(true);

  return (
    <Collapse in={open}>
      <MuiAlert
        {...alertProps}
        sx={{ textAlign: "left", ...alertProps.sx }}
        severity={severity}
        action={
          <>
            {/* CTAs passed in by the consumer */}
            {actions}

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
                  setOpen(false);
                  onDismiss && onDismiss();
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
  );
};

export const AlertDetail = ({ children }: { children: ReactNode }) => {
  return (
    <Box
      component="span"
      color={(theme) => theme.palette.text.secondary}
      fontSize={13}
      data-chromatic="ignore"
    >
      {children}
    </Box>
  );
};
