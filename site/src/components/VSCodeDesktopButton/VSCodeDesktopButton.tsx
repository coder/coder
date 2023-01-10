import Button from "@material-ui/core/Button"
import { getApiKey } from "api/api"
import { VSCodeIcon } from "components/Icons/VSCodeIcon"
import { FC, PropsWithChildren, useState } from "react"

export interface VSCodeDesktopButtonProps {
  userName: string
  workspaceName: string
  agentName?: string
}

export const VSCodeDesktopButton: FC<
  PropsWithChildren<VSCodeDesktopButtonProps>
> = ({ userName, workspaceName, agentName }) => {
  const [loading, setLoading] = useState(false)

  return (
    <Button
      startIcon={<VSCodeIcon />}
      size="small"
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

            window.open(
              `vscode://coder.coder-remote/open?${query.toString()}`,
              "_blank",
            )
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
    </Button>
  )
}
