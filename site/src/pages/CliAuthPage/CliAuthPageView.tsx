import Button from "@mui/material/Button";
import { makeStyles } from "@mui/styles";
import { CodeExample } from "components/CodeExample/CodeExample";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";

export interface CliAuthPageViewProps {
  sessionToken: string | null;
}

export const CliAuthPageView: FC<CliAuthPageViewProps> = ({ sessionToken }) => {
  const styles = useStyles();

  if (!sessionToken) {
    return <FullScreenLoader />;
  }

  return (
    <SignInLayout>
      <Welcome message="Session token" />

      <p className={styles.text}>
        Copy the session token below and{" "}
        <strong className={styles.lineBreak}>paste it in your terminal</strong>.
      </p>

      <CodeExample code={sessionToken} password />

      <div className={styles.links}>
        <Button component={RouterLink} size="large" to="/workspaces" fullWidth>
          Go to workspaces
        </Button>
      </div>
    </SignInLayout>
  );
};

const useStyles = makeStyles((theme) => ({
  title: {
    fontSize: theme.spacing(4),
    fontWeight: 400,
    lineHeight: "140%",
    margin: 0,
  },

  text: {
    fontSize: 16,
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(4),
    textAlign: "center",
    lineHeight: "160%",
  },

  lineBreak: {
    whiteSpace: "nowrap",
  },

  links: {
    display: "flex",
    justifyContent: "flex-end",
    paddingTop: theme.spacing(1),
  },
}));
