import { FC } from "react"
import * as TypesGen from "api/typesGenerated"
import { makeStyles } from "@material-ui/core/styles"
import { useLocation } from "react-router-dom"
import GitHub from "@material-ui/icons/GitHub"
import { GitlabIcon } from "components/Icons/GitlabIcon"
import { AzureDevOpsIcon } from "components/Icons/AzureDevOpsIcon"
import { BitbucketIcon } from "components/Icons/BitbucketIcon"
import { Typography } from "components/Typography/Typography"
import Link from "@material-ui/core/Link"
import Button from "@material-ui/core/Button"
import { Tooltip } from "@material-ui/core"

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
  let icon: JSX.Element
  switch (type) {
    case "azure-devops":
      prettyName = "Azure DevOps"
      icon = <AzureDevOpsIcon />
      break
    case "bitbucket":
      prettyName = "Bitbucket"
      icon = <BitbucketIcon />
      break
    case "github":
      prettyName = "GitHub"
      icon = <GitHub />
      break
    case "gitlab":
      prettyName = "GitLab"
      icon = <GitlabIcon />
      break
  }
  const location = useLocation()
  const redirectURL = `${authenticateURL}?redirect=${location.pathname}`

  return (
    <Tooltip title={authenticated ? "You're already authenticated!" : ""}>
<a href={redirectURL} className={styles.link}>
      <Button disabled={authenticated}>
        <div className={styles.root}>
        {icon}
        {authenticated && <Typography variant="body2">Authenticated with {prettyName}</Typography>}
        {!authenticated && (
          <Typography variant="body2">Authenticate with {prettyName}</Typography>
        )}
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
    display: "flex",
    gap: 16,
    alignItems: "center",
  },
}))
