import Button, { type ButtonProps } from "@mui/material/Button";
import { useTheme } from "@emotion/react";
import { type FC, forwardRef } from "react";

export const PrimaryAgentButton: FC<ButtonProps> = ({ children, ...attrs }) => {
  const theme = useTheme();

  return (
    <Button
      color="neutral"
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
      {...attrs}
    >
      {children}
    </Button>
  );
};

// eslint-disable-next-line react/display-name -- Name is inferred from variable name
export const SecondaryAgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
  (props, ref) => {
    const { children, ...attrs } = props;
    return (
      <Button ref={ref} {...attrs}>
        {children}
      </Button>
    );
  },
);
