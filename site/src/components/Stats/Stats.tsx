import { type CSSObject, type Interpolation, type Theme } from "@emotion/react";
import { type FC, type HTMLAttributes, type ReactNode } from "react";

export const Stats: FC<HTMLAttributes<HTMLDivElement>> = ({
  children,
  ...attrs
}) => {
  return (
    <div css={styles.stats} {...attrs}>
      {children}
    </div>
  );
};

interface StatsItemProps extends HTMLAttributes<HTMLDivElement> {
  label: string;
  value: ReactNode;
}

export const StatsItem: FC<StatsItemProps> = ({ label, value, ...attrs }) => {
  return (
    <div css={styles.statItem} {...attrs}>
      <span css={styles.statsLabel}>{label}:</span>
      <span css={styles.statsValue}>{value}</span>
    </div>
  );
};

const styles = {
  stats: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    paddingLeft: 16,
    paddingRight: 16,
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    margin: "0px",
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      display: "block",
      padding: 16,
    },
  }),

  statItem: (theme) => ({
    padding: 14,
    paddingLeft: 16,
    paddingRight: 16,
    display: "flex",
    alignItems: "baseline",
    gap: 8,

    [theme.breakpoints.down("md")]: {
      padding: 8,
    },
  }),

  statsLabel: {
    display: "block",
    wordWrap: "break-word",
  },

  statsValue: (theme) => ({
    marginTop: 2,
    display: "flex",
    wordWrap: "break-word",
    color: theme.palette.text.primary,
    alignItems: "center",

    "& a": {
      color: theme.palette.text.primary,
      textDecoration: "none",
      fontWeight: 600,

      "&:hover": {
        textDecoration: "underline",
      },
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
