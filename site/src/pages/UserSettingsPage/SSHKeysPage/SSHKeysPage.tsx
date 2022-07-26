import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import { useActor } from "@xstate/react"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import React, { useContext, useEffect } from "react"
import { CodeExample } from "../../../components/CodeExample/CodeExample"
import { ConfirmDialog } from "../../../components/ConfirmDialog/ConfirmDialog"
import { Section } from "../../../components/Section/Section"
import { Stack } from "../../../components/Stack/Stack"
import { XServiceContext } from "../../../xServices/StateContext"

export const Language = {
  title: "SSH keys",
  description:
    "Coder automatically inserts a private key into every workspace; you can add the corresponding public key to any services (such as Git) that you need access to from your workspace.",
  regenerateLabel: "Regenerate",
  regenerateDialogTitle: "Regenerate SSH key?",
  regenerateDialogMessage:
    "You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
  confirmLabel: "Confirm",
  cancelLabel: "Cancel",
  errorRegenerateSSHKey: "Error on regenerate the SSH Key",
}

export const SSHKeysPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { sshKey, getSSHKeyError, regenerateSSHKeyError } = authState.context

  useEffect(() => {
    authSend({ type: "GET_SSH_KEY" })
  }, [authSend])

  return (
    <>
      <Section title={Language.title} description={Language.description}>
        {authState.matches("signedIn.ssh.gettingSSHKey") && (
          <Box p={4}>
            <CircularProgress size={26} />
          </Box>
        )}

        <Stack>
          {/* Regenerating the key is not an option if getSSHKey fails.
           Only one of the error messages will exist at a single time */}
          {getSSHKeyError && <ErrorSummary error={getSSHKeyError} />}
          {regenerateSSHKeyError && (
            <ErrorSummary
              error={regenerateSSHKeyError}
              defaultMessage={Language.errorRegenerateSSHKey}
              dismissible
            />
          )}
          {authState.matches("signedIn.ssh.loaded") && sshKey && (
            <>
              <CodeExample code={sshKey.public_key.trim()} />
              <div>
                <Button
                  variant="outlined"
                  onClick={() => {
                    authSend({ type: "REGENERATE_SSH_KEY" })
                  }}
                >
                  {Language.regenerateLabel}
                </Button>
              </div>
            </>
          )}
        </Stack>
      </Section>

      <ConfirmDialog
        type="delete"
        hideCancel={false}
        open={authState.matches("signedIn.ssh.loaded.confirmSSHKeyRegenerate")}
        confirmLoading={authState.matches("signedIn.ssh.loaded.regeneratingSSHKey")}
        title={Language.regenerateDialogTitle}
        confirmText={Language.confirmLabel}
        onConfirm={() => {
          authSend({ type: "CONFIRM_REGENERATE_SSH_KEY" })
        }}
        onClose={() => {
          authSend({ type: "CANCEL_REGENERATE_SSH_KEY" })
        }}
        description={<>{Language.regenerateDialogMessage}</>}
      />
    </>
  )
}
