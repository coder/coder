import Link from "@material-ui/core/Link"
import Button from "@material-ui/core/Button"
import GitHubIcon from "@material-ui/icons/GitHub"
import KeyIcon from "@material-ui/icons/VpnKey"
import Box from "@material-ui/core/Box"
import { Language } from "./SignInForm"
import { AuthMethods } from "../../api/typesGenerated"
import { FC } from "react"
import { makeStyles } from "@material-ui/core/styles"

type OAuthSignInFormProps = {
  isLoading: boolean
  redirectTo: string
  authMethods?: AuthMethods
}

const useStyles = makeStyles((theme) => ({
  buttonIcon: {
    width: theme.spacing(2),
    height: theme.spacing(2),
  },
}))

export const OAuthSignInForm: FC<OAuthSignInFormProps> = ({
  isLoading,
  redirectTo,
  authMethods,
}) => {
  const styles = useStyles()

  return (
    <Box display="grid" gridGap="16px">
      {authMethods?.github.enabled && (
        <Link
          underline="none"
          href={`/api/v2/users/oauth2/github/callback?redirect=${encodeURIComponent(
            redirectTo,
          )}`}
        >
          <Button
            startIcon={<GitHubIcon className={styles.buttonIcon} />}
            disabled={isLoading}
            fullWidth
            type="submit"
            variant="contained"
          >
            {Language.githubSignIn}
          </Button>
        </Link>
      )}

      {authMethods?.oidc.enabled && (
        <Link
          underline="none"
          href={`/api/v2/users/oidc/callback?redirect=${encodeURIComponent(
            redirectTo,
          )}`}
        >
          <Button
            startIcon={
              authMethods.oidc.iconUrl ? (
                <img
                  alt="Open ID Connect icon"
                  src={authMethods.oidc.iconUrl}
                  className={styles.buttonIcon}
                />
              ) : (
                <KeyIcon className={styles.buttonIcon} />
              )
            }
            disabled={isLoading}
            fullWidth
            type="submit"
            variant="contained"
          >
            {authMethods.oidc.signInText || Language.oidcSignIn}
          </Button>
        </Link>
      )}
    </Box>
  )
}
