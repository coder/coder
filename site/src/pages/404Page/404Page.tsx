import { makeStyles } from "@mui/styles";
import Typography from "@mui/material/Typography";
import { FC } from "react";

export const NotFoundPage: FC = () => {
  const styles = useStyles();

  return (
    <div className={styles.root}>
      <div className={styles.headingContainer}>
        <Typography variant="h4">404</Typography>
      </div>
      <Typography variant="body2">This page could not be found.</Typography>
    </div>
  );
};

const useStyles = makeStyles((theme) => ({
  root: {
    width: "100%",
    height: "100%",
    display: "flex",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  headingContainer: {
    margin: theme.spacing(1),
    padding: theme.spacing(1),
    borderRight: theme.palette.divider,
  },
}));

export default NotFoundPage;
