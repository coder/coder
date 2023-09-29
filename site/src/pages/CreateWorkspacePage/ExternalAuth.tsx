import Button from "@mui/material/Button";
import FormHelperText from "@mui/material/FormHelperText";
import { SvgIconProps } from "@mui/material/SvgIcon";
import Tooltip from "@mui/material/Tooltip";
import GitHub from "@mui/icons-material/GitHub";
import * as TypesGen from "api/typesGenerated";
import { AzureDevOpsIcon } from "components/Icons/AzureDevOpsIcon";
import { BitbucketIcon } from "components/Icons/BitbucketIcon";
import { GitlabIcon } from "components/Icons/GitlabIcon";
import { FC } from "react";
import { makeStyles } from "@mui/styles";
import { type ExternalAuthPollingState } from "./CreateWorkspacePage";
import { Stack } from "components/Stack/Stack";
import ReplayIcon from "@mui/icons-material/Replay";
import { LoadingButton } from "components/LoadingButton/LoadingButton";

export interface ExternalAuthProps {
  type: TypesGen.ExternalAuthProvider;
  authenticated: boolean;
  authenticateURL: string;
  externalAuthPollingState: ExternalAuthPollingState;
  startPollingExternalAuth: () => void;
  error?: string;
}

export const ExternalAuth: FC<ExternalAuthProps> = (props) => {
  const {
    type,
    authenticated,
    authenticateURL,
    externalAuthPollingState,
    startPollingExternalAuth,
    error,
  } = props;

  const styles = useStyles({
    error: typeof error !== "undefined",
  });

  let prettyName: string;
  let Icon: (props: SvgIconProps) => JSX.Element;
  switch (type) {
    case "azure-devops":
      prettyName = "Azure DevOps";
      Icon = AzureDevOpsIcon;
      break;
    case "bitbucket":
      prettyName = "Bitbucket";
      Icon = BitbucketIcon;
      break;
    case "github":
      prettyName = "GitHub";
      Icon = GitHub as (props: SvgIconProps) => JSX.Element;
      break;
    case "gitlab":
      prettyName = "GitLab";
      Icon = GitlabIcon;
      break;
    default:
      throw new Error("invalid git provider: " + type);
  }

  return (
    <Tooltip
      title={authenticated && `${prettyName} has already been connected.`}
    >
      <Stack alignItems="center" spacing={1}>
        <LoadingButton
          loading={externalAuthPollingState === "polling"}
          href={authenticateURL}
          variant="contained"
          size="large"
          startIcon={<Icon />}
          disabled={authenticated}
          className={styles.button}
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
            ? `Authenticated with ${prettyName}`
            : `Login with ${prettyName}`}
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

const useStyles = makeStyles(() => ({
  button: {
    height: 52,
  },
}));
