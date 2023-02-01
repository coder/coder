import { useRef, useState, FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import RefreshIcon from "@material-ui/icons/RefreshOutlined"
import {
  HelpTooltipText,
  HelpPopover,
  HelpTooltipTitle,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipContext,
} from "components/Tooltips/HelpTooltip"
import { WorkspaceAgent } from "api/typesGenerated"
import { getDisplayVersionStatus } from "util/workspace"
import { Stack } from "components/Stack/Stack"

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
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipContext.Provider
          value={{ open: isOpen, onClose: () => setIsOpen(false) }}
        >
          <Stack spacing={1}>
            <div>
              <HelpTooltipTitle>Agent Outdated</HelpTooltipTitle>
              <HelpTooltipText>
                This agent is an older version than the Coder server. This can
                happen after you update Coder with running workspaces. To fix
                this, you can stop and start the workspace.
              </HelpTooltipText>
            </div>

            <Stack spacing={0.5}>
              <span className={styles.versionLabel}>Agent version</span>
              <span>{agent.version}</span>
            </Stack>

            <Stack spacing={0.5}>
              <span className={styles.versionLabel}>Server version</span>
              <span>{serverVersion}</span>
            </Stack>

            <HelpTooltipLinksGroup>
              <HelpTooltipAction
                icon={RefreshIcon}
                onClick={onUpdate}
                ariaLabel="Update workspace"
              >
                Update workspace
              </HelpTooltipAction>
            </HelpTooltipLinksGroup>
          </Stack>
        </HelpTooltipContext.Provider>
      </HelpPopover>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  trigger: {
    cursor: "pointer",
  },

  versionLabel: {
    fontWeight: 600,
    color: theme.palette.text.primary,
  },
}))
