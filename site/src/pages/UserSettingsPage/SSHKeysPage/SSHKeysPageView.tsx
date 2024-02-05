import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import { type FC } from "react";
import { useTheme } from "@emotion/react";
import type { GitSSHKey } from "api/typesGenerated";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Stack } from "components/Stack/Stack";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export interface SSHKeysPageViewProps {
  isLoading: boolean;
  getSSHKeyError?: unknown;
  sshKey?: GitSSHKey;
  onRegenerateClick: () => void;
}

export const SSHKeysPageView: FC<SSHKeysPageViewProps> = ({
  isLoading,
  getSSHKeyError,
  sshKey,
  onRegenerateClick,
}) => {
  const theme = useTheme();

  if (isLoading) {
    return (
      <div css={{ padding: 32 }}>
        <CircularProgress size={26} />
      </div>
    );
  }

  return (
    <Stack>
      {/* Regenerating the key is not an option if getSSHKey fails.
        Only one of the error messages will exist at a single time */}
      {Boolean(getSSHKeyError) && <ErrorAlert error={getSSHKeyError} />}

      {sshKey && (
        <>
          <p
            css={{
              fontSize: 14,
              color: theme.palette.text.secondary,
              margin: 0,
            }}
          >
            The following public key is used to authenticate Git in workspaces.
            You may add it to Git services (such as GitHub) that you need to
            access from your workspace. Coder configures authentication via{" "}
            <code
              css={{
                background: theme.palette.divider,
                fontSize: 12,
                padding: "2px 4px",
                color: theme.palette.text.primary,
                borderRadius: 2,
              }}
            >
              $GIT_SSH_COMMAND
            </code>
            .
          </p>
          <CodeExample secret={false} code={sshKey.public_key.trim()} />
          <div>
            <Button onClick={onRegenerateClick} data-testid="regenerate">
              Regenerate&hellip;
            </Button>
          </div>
        </>
      )}
    </Stack>
  );
};
