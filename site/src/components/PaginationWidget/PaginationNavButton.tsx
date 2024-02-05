import {
  type ButtonHTMLAttributes,
  type ReactNode,
  useEffect,
  useState,
} from "react";
import { useTheme } from "@emotion/react";

import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";

type PaginationNavButtonProps = Omit<
  ButtonHTMLAttributes<HTMLButtonElement>,
  | "aria-disabled"
  // Need to omit color for MUI compatibility
  | "color"
> & {
  // Required/narrowed versions of default props
  children: ReactNode;
  disabled: boolean;
  onClick: () => void;
  "aria-label": string;

  // Bespoke props
  disabledMessage: ReactNode;
  disabledMessageTimeout?: number;
};

function PaginationNavButtonCore({
  onClick,
  disabled,
  disabledMessage,
  disabledMessageTimeout = 3000,
  ...delegatedProps
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

    const timeoutId = setTimeout(
      () => setShowDisabledMessage(false),
      disabledMessageTimeout,
    );

    return () => clearTimeout(timeoutId);
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
        {...delegatedProps}
      />
    </Tooltip>
  );
}

export function PaginationNavButton({
  disabledMessageTimeout = 3000,
  ...delegatedProps
}: PaginationNavButtonProps) {
  return (
    // Key prop ensures that if timeout changes, the component just unmounts and
    // remounts, avoiding a swath of possible sync issues
    <PaginationNavButtonCore
      key={disabledMessageTimeout}
      disabledMessageTimeout={disabledMessageTimeout}
      {...delegatedProps}
    />
  );
}
