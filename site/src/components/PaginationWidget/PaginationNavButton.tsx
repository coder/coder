import { type ReactNode, useEffect, useState } from "react";

import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import { useTheme } from "@emotion/react";

type PaginationNavButtonProps = {
  children: ReactNode;
  disabled: boolean;
  disabledMessage: ReactNode;
  onClick: () => void;
  "aria-label": string;

  disabledMessageTimeout?: number;
};

export function PaginationNavButton({
  children,
  onClick,
  disabled,
  disabledMessage,
  "aria-label": ariaLabel,
  disabledMessageTimeout = 3000,
}: PaginationNavButtonProps) {
  const theme = useTheme();
  const [showDisabledMessage, setShowDisabledMessage] = useState(false);

  // Inline state sync - this is safe/recommended by the React team in this case
  if (!disabled && showDisabledMessage) {
    setShowDisabledMessage(false);
  }

  useEffect(() => {
    if (!showDisabledMessage) {
      return;
    }

    const timeoutId = window.setTimeout(
      () => setShowDisabledMessage(false),
      disabledMessageTimeout,
    );

    return () => {
      setShowDisabledMessage(false);
      window.clearTimeout(timeoutId);
    };
  }, [showDisabledMessage, disabledMessageTimeout]);

  return (
    <Tooltip title={disabledMessage} open={showDisabledMessage}>
      {/*
       * Going more out of the way to avoid attaching the disabled prop directly
       * to avoid unwanted side effects of using the prop:
       * - Not being focusable/keyboard-navigable
       * - Not being able to call functions in response to invalid actions
       *   (mostly for giving direct UI feedback to those actions)
       */}
      <Button
        aria-label={ariaLabel}
        aria-disabled={disabled}
        css={
          disabled && {
            borderColor: theme.palette.divider,
            color: theme.palette.text.disabled,
            cursor: "default",
            "&:hover": {
              backgroundColor: theme.palette.background.default,
              borderColor: theme.palette.divider,
            },
          }
        }
        onClick={() => {
          if (disabled) {
            setShowDisabledMessage(true);
          } else {
            onClick();
          }
        }}
      >
        {children}
      </Button>
    </Tooltip>
  );
}
