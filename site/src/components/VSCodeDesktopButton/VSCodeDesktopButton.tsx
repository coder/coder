import { getApiKey } from "api/api"
import { VSCodeIcon } from "components/Icons/VSCodeIcon"
import { FC, PropsWithChildren, useState } from "react"
import { PrimaryAgentButton } from "components/Resources/AgentButton"

export interface VSCodeDesktopButtonProps {
  userName: string
  workspaceName: string
  agentName?: string
  folderPath?: string
}

export const VSCodeDesktopButton: FC<
  PropsWithChildren<VSCodeDesktopButtonProps>
> = ({ userName, workspaceName, agentName, folderPath }) => {
  const [loading, setLoading] = useState(false)

  return (
    <PrimaryAgentButton
      startIcon={<VSCodeIcon />}
      disabled={loading}
      onClick={() => {
        setLoading(true)
        getApiKey()
          .then(({ key }) => {
            const query = new URLSearchParams({
              owner: userName,
              workspace: workspaceName,
              url: location.origin,
              token: key,
            })
            if (agentName) {
              query.set("agent", agentName)
            }
            if (folderPath) {
              query.set("folder", folderPath)
            }

            location.href = `vscode://coder.coder-remote/open?${query.toString()}`
          })
          .catch((ex) => {
            console.error(ex)
          })
          .finally(() => {
            setLoading(false)
          })
      }}
    >
      VS Code Desktop
    </PrimaryAgentButton>
  )
}
