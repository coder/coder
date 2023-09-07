import CircularProgress from "@mui/material/CircularProgress";
import { makeStyles } from "@mui/styles";
import { FC } from "react";

export const useStyles = makeStyles((theme) => ({
  root: {
    position: "absolute",
    top: "0",
    left: "0",
    right: "0",
    bottom: "0",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
    background: theme.palette.background.default,
  },
}));

export const FullScreenLoader: FC = () => {
  const styles = useStyles();

  return (
    <div className={styles.root} data-testid="loader">
      <CircularProgress />
    </div>
  );
};
