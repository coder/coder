import Button from "@mui/material/Button";
import { type Interpolation, type Theme } from "@emotion/react";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { CodeExample } from "components/CodeExample/CodeExample";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";

export interface CliAuthPageViewProps {
  sessionToken?: string;
}

export const CliAuthPageView: FC<CliAuthPageViewProps> = ({ sessionToken }) => {
  if (!sessionToken) {
    return <FullScreenLoader />;
  }

  return (
    <SignInLayout>
      <Welcome>Session token</Welcome>

      <p css={styles.instructions}>
        Copy the session token below and{" "}
        <strong css={{ whiteSpace: "nowrap" }}>
          paste it in your terminal
        </strong>
        .
      </p>

      <CodeExample code={sessionToken} secret />

      <div css={styles.backButton}>
        <Button component={RouterLink} size="large" to="/workspaces" fullWidth>
          Go to workspaces
        </Button>
      </div>
    </SignInLayout>
  );
};

const styles = {
  instructions: (theme) => ({
    fontSize: 16,
    color: theme.palette.text.secondary,
    marginBottom: 32,
    textAlign: "center",
    lineHeight: "160%",
  }),

  backButton: {
    display: "flex",
    justifyContent: "flex-end",
    paddingTop: 8,
  },
} satisfies Record<string, Interpolation<Theme>>;
