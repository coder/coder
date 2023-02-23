import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { SvgIconProps } from "@material-ui/core/SvgIcon"
import Tooltip from "@material-ui/core/Tooltip"
import GitHub from "@material-ui/icons/GitHub"
import * as TypesGen from "api/typesGenerated"
import { AzureDevOpsIcon } from "components/Icons/AzureDevOpsIcon"
import { BitbucketIcon } from "components/Icons/BitbucketIcon"
import { GitlabIcon } from "components/Icons/GitlabIcon"
import { Typography } from "components/Typography/Typography"
import { FC } from "react"

export interface GitAuthProps {
  type: TypesGen.GitProvider
  authenticated: boolean
  authenticateURL: string
}

export const GitAuth: FC<GitAuthProps> = ({
  type,
  authenticated,
  authenticateURL,
}) => {
  const styles = useStyles()

  let prettyName: string
  let Icon: (props: SvgIconProps) => JSX.Element
  switch (type) {
    case "azure-devops":
      prettyName = "Azure DevOps"
      Icon = AzureDevOpsIcon
      break
    case "bitbucket":
      prettyName = "Bitbucket"
      Icon = BitbucketIcon
      break
    case "github":
      prettyName = "GitHub"
      Icon = GitHub
      break
    case "gitlab":
      prettyName = "GitLab"
      Icon = GitlabIcon
      break
    default:
      throw new Error("invalid git provider: " + type)
  }
  // The Git auth page uses a BroadcastChannel to notify listening
  // pages for Git auth updates if the "notify" query parameter is specified.
  const redirectURL = `${authenticateURL}?redirect=${encodeURIComponent(
    `/gitauth?notify`,
  )}`

  return (
    <Tooltip
      title={
        authenticated ? "You're already authenticated! No action needed." : ``
      }
    >
      <a
        href={redirectURL}
        className={styles.link}
        onClick={(event) => {
          event.preventDefault()
          // If the user is already authenticated, we don't want to redirect them
          if (authenticated || authenticateURL === "") {
            return
          }
          window.open(redirectURL, "_blank", "width=900,height=600")
        }}
      >
        <Button className={styles.button} disabled={authenticated} fullWidth>
          <div className={styles.root}>
            <Icon className={styles.icon} />
            <div className={styles.text}>
              <Typography variant="body2">
                {authenticated
                  ? `You're authenticated with ${prettyName}!`
                  : `Click to login with ${prettyName}!`}
              </Typography>
              {!authenticated && (
                <Typography variant="caption" color="textSecondary">
                  {"You'll be redirected back here once authenticated."}
                </Typography>
              )}
            </div>
          </div>
        </Button>
      </a>
    </Tooltip>
  )
}

const useStyles = makeStyles(() => ({
  link: {
    textDecoration: "none",
  },
  root: {
    padding: 4,
    display: "flex",
    gap: 12,
    alignItems: "center",
    textAlign: "left",
  },
  button: {
    height: "unset",
  },
  text: {
    display: "flex",
    flexDirection: "column",
    gap: 4,
  },
  icon: {
    width: 32,
    height: 32,
  },
}))
