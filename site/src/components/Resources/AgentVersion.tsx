import { useRef, useState, FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import {
  HelpTooltipText,
  HelpPopover,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip"
import { WorkspaceAgent } from "api/typesGenerated"
import { getDisplayVersionStatus } from "util/workspace"

export const AgentVersion: FC<{
  agent: WorkspaceAgent
  serverVersion: string
}> = ({ agent, serverVersion }) => {
  const styles = useStyles()
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "version-outdated-popover" : undefined
  const { displayVersion, outdated } = getDisplayVersionStatus(
    agent.version,
    serverVersion,
  )

  if (!outdated) {
    return <span>{displayVersion}</span>
  }

  return (
    <>
      <span
        role="presentation"
        aria-label="latency"
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        className={styles.trigger}
      >
        Agent Outdated
      </span>
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>Agent Outdated</HelpTooltipTitle>
        <HelpTooltipText>
          This agent is an older version than the Coder server. This can happen
          after you update Coder with running workspaces. To fix this, you can
          stop and start the workspace.
        </HelpTooltipText>
      </HelpPopover>
    </>
  )
}

const useStyles = makeStyles(() => ({
  trigger: {
    cursor: "pointer",
  },
}))
