import type { Interpolation, Theme } from "@emotion/react";
import CircularProgress from "@mui/material/CircularProgress";
import type { FC, HTMLAttributes } from "react";

interface LoaderProps extends HTMLAttributes<HTMLDivElement> {
  fullscreen?: boolean;
  size?: number;
}

export const Loader: FC<LoaderProps> = ({
  fullscreen,
  size = 26,
  ...attrs
}) => {
  return (
    <div
      css={fullscreen ? styles.fullscreen : styles.inline}
      data-testid="loader"
      {...attrs}
    >
      <CircularProgress size={size} />
    </div>
  );
};

const styles = {
  inline: {
    padding: 32,
    width: "100%",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },
  fullscreen: (theme) => ({
    position: "absolute",
    top: "0",
    left: "0",
    right: "0",
    bottom: "0",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
    background: theme.palette.background.default,
  }),
} satisfies Record<string, Interpolation<Theme>>;
