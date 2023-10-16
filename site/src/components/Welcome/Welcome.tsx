import { makeStyles } from "@mui/styles";
import Typography from "@mui/material/Typography";
import { FC, PropsWithChildren } from "react";
import { CoderIcon } from "../Icons/CoderIcon";

const Language = {
  defaultMessage: (
    <>
      Welcome to <strong>Coder</strong>
    </>
  ),
};

export const Welcome: FC<
  PropsWithChildren<{ message?: JSX.Element | string }>
> = ({ message = Language.defaultMessage }) => {
  const styles = useStyles();

  return (
    <div>
      <div className={styles.logoBox}>
        <CoderIcon className={styles.logo} />
      </div>
      <Typography className={styles.title} variant="h1">
        {message}
      </Typography>
    </div>
  );
};

const useStyles = makeStyles((theme) => ({
  logoBox: {
    display: "flex",
    justifyContent: "center",
  },
  logo: {
    color: theme.palette.text.primary,
    fontSize: theme.spacing(8),
  },
  title: {
    textAlign: "center",
    fontSize: theme.spacing(4),
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(2),
    marginTop: theme.spacing(2),
    lineHeight: 1.25,

    "& strong": {
      fontWeight: 600,
    },
  },
}));
