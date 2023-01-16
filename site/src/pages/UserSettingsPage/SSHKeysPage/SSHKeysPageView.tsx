import { makeStyles } from "@material-ui/core/styles"
import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import { GitSSHKey } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"

export const Language = {
  errorRegenerateSSHKey: "Error on regenerating the SSH Key",
  regenerateLabel: "Regenerate",
}

export interface SSHKeysPageViewProps {
  isLoading: boolean
  hasLoaded: boolean
  getSSHKeyError?: Error | unknown
  regenerateSSHKeyError?: Error | unknown
  sshKey?: GitSSHKey
  onRegenerateClick: () => void
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
  const styles = useStyles()

  if (isLoading) {
    return (
      <Box p={4}>
        <CircularProgress size={26} />
      </Box>
    )
  }

  return (
    <Stack>
      {/* Regenerating the key is not an option if getSSHKey fails.
        Only one of the error messages will exist at a single time */}
      {Boolean(getSSHKeyError) && (
        <AlertBanner severity="error" error={getSSHKeyError} />
      )}
      {Boolean(regenerateSSHKeyError) && (
        <AlertBanner
          severity="error"
          error={regenerateSSHKeyError}
          text={Language.errorRegenerateSSHKey}
          dismissible
        />
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
  )
}

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
}))
