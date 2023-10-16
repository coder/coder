import { makeStyles } from "@mui/styles";
import { FC, ReactNode } from "react";

export const useStyles = makeStyles((theme) => ({
  root: {
    flex: 1,
    height: "-webkit-fill-available",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
  layout: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },
  container: {
    maxWidth: 385,
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
  },
  footer: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(3),
  },
}));

export const SignInLayout: FC<{ children: ReactNode }> = ({ children }) => {
  const styles = useStyles();

  return (
    <div className={styles.root}>
      <div className={styles.layout}>
        <div className={styles.container}>{children}</div>
        <div className={styles.footer}>
          {`\u00a9 ${new Date().getFullYear()} Coder Technologies, Inc.`}
        </div>
      </div>
    </div>
  );
};
