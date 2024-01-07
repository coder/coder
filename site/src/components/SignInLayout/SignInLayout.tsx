import { type Interpolation, type Theme } from "@emotion/react";
import { type FC, type PropsWithChildren } from "react";

export const SignInLayout: FC<PropsWithChildren> = ({ children }) => {
  return (
    <div css={styles.container}>
      <div css={styles.content}>
        <div css={styles.signIn}>{children}</div>
        <div css={styles.copyright}>
          {"\u00a9"} {new Date().getFullYear()} Coder Technologies, Inc.
        </div>
      </div>
    </div>
  );
};

const styles = {
  container: {
    flex: 1,
    height: "-webkit-fill-available",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },

  content: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },

  signIn: {
    maxWidth: 385,
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },

  copyright: (theme) => ({
    fontSize: 12,
    color: theme.palette.text.secondary,
    marginTop: 24,
  }),
} satisfies Record<string, Interpolation<Theme>>;
