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

export interface GitAuthProps {
  type: TypesGen.GitProvider;
  authenticated: boolean;
  authenticateURL: string;
  error?: string;
}

export const GitAuth: FC<GitAuthProps> = ({
  type,
  authenticated,
  authenticateURL,
  error,
}) => {
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
      title={
        authenticated ? "You're already authenticated! No action needed." : ``
      }
    >
      <div>
        <Button
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
          }}
        >
          {authenticated
            ? `You're authenticated with ${prettyName}!`
            : `Click to login with ${prettyName}!`}
        </Button>

        {error && <FormHelperText error>{error}</FormHelperText>}
      </div>
    </Tooltip>
  );
};

const useStyles = makeStyles(() => ({
  button: {
    height: 52,
  },
}));
