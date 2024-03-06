import type { Interpolation, Theme } from "@emotion/react";
import type { FC, PropsWithChildren } from "react";
import { CoderIcon } from "../Icons/CoderIcon";

const Language = {
  defaultMessage: (
    <>
      Welcome to <strong>Coder</strong>
    </>
  ),
};

export const Welcome: FC<PropsWithChildren> = ({ children }) => {
  return (
    <div>
      <div css={styles.container}>
        <CoderIcon css={styles.icon} />
      </div>
      <h1 css={styles.header}>{children || Language.defaultMessage}</h1>
    </div>
  );
};

const styles = {
  container: {
    display: "flex",
    justifyContent: "center",
  },

  icon: (theme) => ({
    color: theme.palette.text.primary,
    fontSize: 64,
  }),

  header: {
    textAlign: "center",
    fontSize: 32,
    fontWeight: 400,
    margin: 0,
    marginTop: 16,
    marginBottom: 32,
    lineHeight: 1.25,

    "& strong": {
      fontWeight: 600,
    },
  },
} satisfies Record<string, Interpolation<Theme>>;
