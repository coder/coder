import Button, { type ButtonProps } from "@mui/material/Button";
import { forwardRef } from "react";

// eslint-disable-next-line react/display-name -- Name is inferred from variable name
export const AgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
  (props, ref) => {
    const { children, ...buttonProps } = props;

    return (
      <Button
        {...buttonProps}
        color="neutral"
        size="large"
        variant="contained"
        ref={ref}
        css={(theme) => ({
          height: 44,
          padding: "12px 20px",
          color: theme.palette.text.primary,
          // Making them smaller since those icons don't have a padding around them
          "& .MuiButton-startIcon": {
            width: 16,
            height: 16,
            marginRight: 12,
            "& svg": { width: "100%", height: "100%" },
          },
        })}
      >
        {children}
      </Button>
    );
  },
);
