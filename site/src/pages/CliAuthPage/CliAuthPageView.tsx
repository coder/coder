import Button from "@mui/material/Button";
import { useTheme } from "@emotion/react";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { CodeExample } from "components/CodeExample/CodeExample";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";

export interface CliAuthPageViewProps {
  sessionToken: string | null;
}

export const CliAuthPageView: FC<CliAuthPageViewProps> = ({ sessionToken }) => {
  const theme = useTheme();

  if (!sessionToken) {
    return <FullScreenLoader />;
  }

  return (
    <SignInLayout>
      <Welcome>Session token</Welcome>

      <p
        css={{
          fontSize: 16,
          color: theme.palette.text.secondary,
          marginBottom: 32,
          textAlign: "center",
          lineHeight: "160%",
        }}
      >
        Copy the session token below and{" "}
        <strong css={{ whiteSpace: "nowrap" }}>
          paste it in your terminal
        </strong>
        .
      </p>

      <CodeExample code={sessionToken} secret />

      <div
        css={{
          display: "flex",
          justifyContent: "flex-end",
          paddingTop: 8,
        }}
      >
        <Button component={RouterLink} size="large" to="/workspaces" fullWidth>
          Go to workspaces
        </Button>
      </div>
    </SignInLayout>
  );
};
