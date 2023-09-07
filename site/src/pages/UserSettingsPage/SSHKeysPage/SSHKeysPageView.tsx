import { makeStyles } from "@mui/styles";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import { GitSSHKey } from "api/typesGenerated";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Stack } from "components/Stack/Stack";
import { FC } from "react";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export const Language = {
  regenerateLabel: "Regenerate",
};

export interface SSHKeysPageViewProps {
  isLoading: boolean;
  hasLoaded: boolean;
  getSSHKeyError?: unknown;
  regenerateSSHKeyError?: unknown;
  sshKey?: GitSSHKey;
  onRegenerateClick: () => void;
}

export const SSHKeysPageView: FC<
  React.PropsWithChildren<SSHKeysPageViewProps>
> = ({
  isLoading,
  hasLoaded,
  getSSHKeyError,
  regenerateSSHKeyError,
  sshKey,
  onRegenerateClick,
}) => {
  const styles = useStyles();

  if (isLoading) {
    return (
      <Box p={4}>
        <CircularProgress size={26} />
      </Box>
    );
  }

  return (
    <Stack>
      {/* Regenerating the key is not an option if getSSHKey fails.
        Only one of the error messages will exist at a single time */}
      {Boolean(getSSHKeyError) && <ErrorAlert error={getSSHKeyError} />}
      {Boolean(regenerateSSHKeyError) && (
        <ErrorAlert error={regenerateSSHKeyError} dismissible />
      )}
      {hasLoaded && sshKey && (
        <>
          <p className={styles.description}>
            The following public key is used to authenticate Git in workspaces.
            You may add it to Git services (such as GitHub) that you need to
            access from your workspace. Coder configures authentication via{" "}
            <code className={styles.code}>$GIT_SSH_COMMAND</code>.
          </p>
          <CodeExample code={sshKey.public_key.trim()} />
          <div>
            <Button onClick={onRegenerateClick}>
              {Language.regenerateLabel}
            </Button>
          </div>
        </>
      )}
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  description: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    margin: 0,
  },
  code: {
    background: theme.palette.divider,
    fontSize: 12,
    padding: "2px 4px",
    color: theme.palette.text.primary,
    borderRadius: 2,
  },
}));
