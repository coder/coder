import { useActor } from "@xstate/react"
import React, { useContext, useEffect } from "react"
import { ConfirmDialog } from "../../../components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Section } from "../../../components/Section/Section"
import { XServiceContext } from "../../../xServices/StateContext"
import { SSHKeysPageView } from "./SSHKeysPageView"

export const Language = {
  title: "SSH keys",
  description: (
    <p>
      The following public key is used to authenticate Git in workspaces. You may add it to Git
      services (such as GitHub) that you need to access from your workspace. <br />
      <br />
      Coder configures authentication via <code>$GIT_SSH_COMMAND</code>.
    </p>
  ),
  regenerateDialogTitle: "Regenerate SSH key?",
  regenerateDialogMessage:
    "You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
  confirmLabel: "Confirm",
  cancelLabel: "Cancel",
}

export const SSHKeysPage: React.FC<React.PropsWithChildren<unknown>> = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { sshKey, getSSHKeyError, regenerateSSHKeyError } = authState.context

  useEffect(() => {
    authSend({ type: "GET_SSH_KEY" })
  }, [authSend])

  const isLoading = authState.matches("signedIn.ssh.gettingSSHKey")
  const hasLoaded = authState.matches("signedIn.ssh.loaded")

  const onRegenerateClick = () => {
    authSend({ type: "REGENERATE_SSH_KEY" })
  }

  return (
    <>
      <Section title={Language.title} description={Language.description}>
        <SSHKeysPageView
          isLoading={isLoading}
          hasLoaded={hasLoaded}
          getSSHKeyError={getSSHKeyError}
          regenerateSSHKeyError={regenerateSSHKeyError}
          sshKey={sshKey}
          onRegenerateClick={onRegenerateClick}
        />
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

export default SSHKeysPage
