import { type Interpolation, type Theme } from "@emotion/react";
import { type FC, type PropsWithChildren } from "react";

export const DividerWithText: FC<PropsWithChildren> = ({ children }) => {
  return (
    <div css={styles.container}>
      <div css={styles.border} />
      <span css={styles.content}>{children}</span>
      <div css={styles.border} />
    </div>
  );
};

const styles = {
  container: {
    display: "flex",
    alignItems: "center",
  },
  border: (theme) => ({
    borderBottom: `2px solid ${theme.palette.divider}`,
    width: "100%",
  }),
  content: (theme) => ({
    paddingTop: theme.spacing(0.5),
    paddingBottom: theme.spacing(0.5),
    paddingRight: theme.spacing(2),
    paddingLeft: theme.spacing(2),
    fontSize: theme.typography.h5.fontSize,
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
