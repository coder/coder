import { useActor } from "@xstate/react"
import { useContext, useEffect, PropsWithChildren, FC } from "react"
import { ConfirmDialog } from "../../../components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Section } from "../../../components/SettingsLayout/Section"
import { XServiceContext } from "../../../xServices/StateContext"
import { SSHKeysPageView } from "./SSHKeysPageView"

export const Language = {
  title: "SSH keys",
  regenerateDialogTitle: "Regenerate SSH key?",
  regenerateDialogMessage:
    "You will need to replace the public SSH key on services you use it with, and you'll need to rebuild existing workspaces.",
  confirmLabel: "Confirm",
  cancelLabel: "Cancel",
}

export const SSHKeysPage: FC<PropsWithChildren<unknown>> = () => {
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
      <Section title={Language.title}>
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
        confirmLoading={authState.matches(
          "signedIn.ssh.loaded.regeneratingSSHKey",
        )}
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
