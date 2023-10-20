import ReplayIcon from "@mui/icons-material/Replay";
import Button from "@mui/material/Button";
import FormHelperText from "@mui/material/FormHelperText";
import Tooltip from "@mui/material/Tooltip";
import { type FC } from "react";
import { Stack } from "components/Stack/Stack";
import { type ExternalAuthPollingState } from "./CreateWorkspacePage";
import LoadingButton from "@mui/lab/LoadingButton";

export interface ExternalAuthProps {
  displayName: string;
  displayIcon: string;
  authenticated: boolean;
  authenticateURL: string;
  externalAuthPollingState: ExternalAuthPollingState;
  startPollingExternalAuth: () => void;
  error?: string;
}

export const ExternalAuth: FC<ExternalAuthProps> = (props) => {
  const {
    displayName,
    displayIcon,
    authenticated,
    authenticateURL,
    externalAuthPollingState,
    startPollingExternalAuth,
    error,
  } = props;

  return (
    <Tooltip
      title={authenticated && `${displayName} has already been connected.`}
    >
      <Stack alignItems="center" spacing={1}>
        <LoadingButton
          loadingPosition="start"
          loading={externalAuthPollingState === "polling"}
          href={authenticateURL}
          variant="contained"
          size="large"
          startIcon={
            <img
              src={displayIcon}
              alt={`${displayName} Icon`}
              width={16}
              height={16}
            />
          }
          disabled={authenticated}
          css={{ height: 52 }}
          color={error ? "error" : undefined}
          fullWidth
          onClick={(event) => {
            event.preventDefault();
            // If the user is already authenticated, we don't want to redirect them
            if (authenticated || authenticateURL === "") {
              return;
            }
            window.open(authenticateURL, "_blank", "width=900,height=600");
            startPollingExternalAuth();
          }}
        >
          {authenticated
            ? `Authenticated with ${displayName}`
            : `Login with ${displayName}`}
        </LoadingButton>

        {externalAuthPollingState === "abandoned" && (
          <Button variant="text" onClick={startPollingExternalAuth}>
            <ReplayIcon /> Check again
          </Button>
        )}
        {error && <FormHelperText error>{error}</FormHelperText>}
      </Stack>
    </Tooltip>
  );
};
