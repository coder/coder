import Button, { ButtonProps } from "@mui/material/Button";
import IconButton, { IconButtonProps } from "@mui/material/IconButton";
import { useTheme } from "@mui/material/styles";
import { Avatar, AvatarProps } from "components/Avatar/Avatar";
import { HTMLAttributes, forwardRef } from "react";

export const Topbar = (props: HTMLAttributes<HTMLDivElement>) => {
  const theme = useTheme();

  return (
    <header
      {...props}
      css={{
        height: 48,
        borderBottom: `1px solid ${theme.palette.divider}`,
        display: "flex",
        alignItems: "center",
        fontSize: 13,
        lineHeight: "1.2",
      }}
    />
  );
};

export const TopbarIconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  (props, ref) => {
    return (
      <IconButton
        ref={ref}
        {...props}
        size="small"
        css={{
          padding: "0 16px",
          borderRadius: 0,
          height: 48,

          "& svg": {
            fontSize: 20,
          },
        }}
      />
    );
  },
) as typeof IconButton;

export const TopbarButton = (props: ButtonProps) => {
  return (
    <Button
      {...props}
      css={{
        height: 28,
        fontSize: 13,
        borderRadius: 4,
        padding: "0 12px",
      }}
    />
  );
};

export const TopbarData = (props: HTMLAttributes<HTMLDivElement>) => {
  return (
    <div
      {...props}
      css={{
        display: "flex",
        gap: 8,
        alignItems: "center",
        justifyContent: "center",
      }}
    />
  );
};

export const TopbarDivider = (props: HTMLAttributes<HTMLSpanElement>) => {
  const theme = useTheme();
  return (
    <span {...props} css={{ color: theme.palette.divider }}>
      /
    </span>
  );
};

export const TopbarAvatar = (props: AvatarProps) => {
  return (
    <Avatar
      {...props}
      variant="square"
      fitImage
      css={{ width: 16, height: 16 }}
    />
  );
};
