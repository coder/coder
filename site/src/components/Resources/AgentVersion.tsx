import { useRef, useState, FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { WorkspaceAgent } from "api/typesGenerated"
import { getDisplayVersionStatus } from "util/workspace"
import { AgentOutdatedTooltip } from "components/Tooltips/AgentOutdatedTooltip"

export const AgentVersion: FC<{
  agent: WorkspaceAgent
  serverVersion: string
  onUpdate: () => void
}> = ({ agent, serverVersion, onUpdate }) => {
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
        onMouseLeave={() => setIsOpen(false)}
        className={styles.trigger}
      >
        Agent Outdated
      </span>
      <AgentOutdatedTooltip
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
        agent={agent}
        serverVersion={serverVersion}
        onUpdate={onUpdate}
      />
    </>
  )
}

const useStyles = makeStyles(() => ({
  trigger: {
    cursor: "pointer",
  },
}))
