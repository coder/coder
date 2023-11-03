// This is the only place MuiAvatar can be used
// eslint-disable-next-line no-restricted-imports -- Read above
import MuiAvatar, { AvatarProps as MuiAvatarProps } from "@mui/material/Avatar";
import { FC } from "react";
import { css, type Interpolation, type Theme } from "@emotion/react";

export type AvatarProps = MuiAvatarProps & {
  size?: "xs" | "sm" | "md" | "xl";
  colorScheme?: "light" | "darken";
  fitImage?: boolean;
};

const sizeStyles = {
  xs: {
    width: 16,
    height: 16,
    fontSize: 8,
    fontWeight: 700,
  },
  sm: {
    width: 24,
    height: 24,
    fontSize: 12,
    fontWeight: 600,
  },
  md: {},
  xl: {
    width: 48,
    height: 48,
    fontSize: 24,
  },
} satisfies Record<string, Interpolation<Theme>>;

const colorStyles = {
  light: {},
  darken: (theme) => ({
    background: theme.palette.divider,
    color: theme.palette.text.primary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

const fitImageStyles = css`
  & .MuiAvatar-img {
    object-fit: contain;
  }
`;

export const Avatar: FC<AvatarProps> = ({
  size = "md",
  colorScheme = "light",
  fitImage,
  children,
  ...muiProps
}) => {
  return (
    <MuiAvatar
      {...muiProps}
      css={[
        sizeStyles[size],
        colorStyles[colorScheme],
        fitImage && fitImageStyles,
      ]}
    >
      {typeof children === "string" ? firstLetter(children) : children}
    </MuiAvatar>
  );
};

/**
 * Use it to make an img element behaves like a MaterialUI Icon component
 */
export const AvatarIcon: FC<{ src: string }> = ({ src }) => {
  return (
    <img
      src={src}
      alt=""
      css={{
        maxWidth: "50%",
      }}
    />
  );
};

const firstLetter = (str: string): string => {
  if (str.length > 0) {
    return str[0].toLocaleUpperCase();
  }

  return "";
};
