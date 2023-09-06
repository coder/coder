import { makeStyles } from "@mui/styles";
import { FC, PropsWithChildren } from "react";
import { combineClasses } from "utils/combineClasses";

export const FullWidthPageHeader: FC<
  PropsWithChildren & { sticky?: boolean }
> = ({ children, sticky = true }) => {
  const styles = useStyles();

  return (
    <header
      className={combineClasses([styles.header, sticky ? styles.sticky : ""])}
      data-testid="header"
    >
      {children}
    </header>
  );
};

export const PageHeaderActions: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return <div className={styles.actions}>{children}</div>;
};

export const PageHeaderTitle: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return <h1 className={styles.title}>{children}</h1>;
};

export const PageHeaderSubtitle: FC<PropsWithChildren> = ({ children }) => {
  const styles = useStyles();
  return <span className={styles.subtitle}>{children}</span>;
};

const useStyles = makeStyles((theme) => ({
  header: {
    ...theme.typography.body2,
    padding: theme.spacing(3),
    background: theme.palette.background.paper,
    borderBottom: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(6),

    zIndex: 10,
    flexWrap: "wrap",

    [theme.breakpoints.down("lg")]: {
      position: "unset",
      alignItems: "flex-start",
    },
    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
    },
  },
  sticky: {
    position: "sticky",
    top: 0,
  },
  actions: {
    marginLeft: "auto",
    [theme.breakpoints.down("md")]: {
      marginLeft: "unset",
    },
  },
  title: {
    fontSize: 18,
    fontWeight: 500,
    margin: 0,
    lineHeight: "24px",
  },
  subtitle: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    display: "block",
  },
}));
