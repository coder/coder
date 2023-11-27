import Button, { ButtonProps } from "@mui/material/Button";
import { forwardRef } from "react";

// eslint-disable-next-line react/display-name -- Name is inferred from variable name
export const AgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
  (props, ref) => {
    return (
      <Button
        color="neutral"
        {...props}
        ref={ref}
        sx={{
          backgroundColor: (theme) => theme.palette.background.default,
          "&:hover": {
            backgroundColor: (theme) => theme.palette.background.paper,
          },
          // Making them smaller since those icons don't have a padding around them
          "& .MuiButton-startIcon": {
            width: 12,
            height: 12,
            "& svg": { width: "100%", height: "100%" },
          },
          ...props.sx,
        }}
      />
    );
  },
);
