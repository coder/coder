import Button, { ButtonProps } from "@mui/material/Button";
import { FC, forwardRef } from "react";

export const PrimaryAgentButton: FC<ButtonProps> = ({
  className,
  ...props
}) => {
  return (
    <Button
      color="neutral"
      {...props}
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
};

// eslint-disable-next-line react/display-name -- Name is inferred from variable name
export const SecondaryAgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, ...props }, ref) => {
    return <Button ref={ref} className={className} {...props} />;
  },
);
