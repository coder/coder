import Button, { type ButtonProps } from "@mui/material/Button";
import { useTheme } from "@emotion/react";
import { forwardRef } from "react";

// eslint-disable-next-line react/display-name -- Name is inferred from variable name
export const AgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
  (props, ref) => {
    const { children, ...buttonProps } = props;
    const theme = useTheme();

    return (
      <Button
        color="neutral"
        {...buttonProps}
        ref={ref}
        css={{
          backgroundColor: theme.palette.background.default,

          "&:hover": {
            backgroundColor: theme.palette.background.paper,
          },

          // Making them smaller since those icons don't have a padding around them
          "& .MuiButton-startIcon": {
            width: 12,
            height: 12,
            "& svg": { width: "100%", height: "100%" },
          },
        }}
      >
        {children}
      </Button>
    );
  },
);
