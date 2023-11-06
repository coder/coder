import { type CSSObject, type Interpolation, type Theme } from "@emotion/react";
import Box from "@mui/material/Box";
import { ReactNode, type ComponentProps, type FC } from "react";

export const Stats: FC<ComponentProps<typeof Box>> = (props) => {
  return <Box {...props} css={styles.stats} />;
};

export const StatsItem: FC<
  {
    label: string;
    value: ReactNode;
  } & ComponentProps<typeof Box>
> = ({ label, value, ...divProps }) => {
  return (
    <Box {...divProps} css={styles.statItem}>
      <span css={styles.statsLabel}>{label}:</span>
      <span css={styles.statsValue}>{value}</span>
    </Box>
  );
};

const styles = {
  stats: (theme) => ({
    ...(theme.typography.body2 as CSSObject),
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    margin: "0px",
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      display: "block",
      padding: theme.spacing(2),
    },
  }),

  statItem: (theme) => ({
    padding: theme.spacing(1.75),
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    display: "flex",
    alignItems: "baseline",
    gap: theme.spacing(1),

    [theme.breakpoints.down("md")]: {
      padding: theme.spacing(1),
    },
  }),

  statsLabel: {
    display: "block",
    wordWrap: "break-word",
  },

  statsValue: (theme) => ({
    marginTop: theme.spacing(0.25),
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
