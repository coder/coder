import Button from "@material-ui/core/Button"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import CloudIcon from "@material-ui/icons/CloudOutlined"
import { useRef, useState } from "react"
import { CodeExample } from "../CodeExample/CodeExample"
import { Stack } from "../Stack/Stack"
import { HelpTooltipLink, HelpTooltipLinksGroup, HelpTooltipText } from "../Tooltips/HelpTooltip"

export interface SSHButtonProps {
  workspaceName: string
  agentName: string
  defaultIsOpen?: boolean
}

export const SSHButton: React.FC<React.PropsWithChildren<SSHButtonProps>> = ({
  workspaceName,
  agentName,
  defaultIsOpen = false,
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(defaultIsOpen)
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles()

  const onClose = () => {
    setIsOpen(false)
  }

  return (
    <>
      <Button
        startIcon={<CloudIcon />}
        size="small"
        ref={anchorRef}
        onClick={() => {
          setIsOpen(true)
        }}
      >
        SSH
      </Button>
      <Popover
        classes={{ paper: styles.popoverPaper }}
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={onClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
      >
        <HelpTooltipText>Run the following commands to connect with SSH:</HelpTooltipText>

        <Stack spacing={0.5} className={styles.codeExamples}>
          <div>
            <HelpTooltipText>
              <strong className={styles.codeExampleLabel}>
                Configure ssh{" "}
                <span className={styles.textHelper}>
                  - only needs to be run once, or after managing workspaces
                </span>
              </strong>
            </HelpTooltipText>
            <CodeExample code="coder config-ssh" />
          </div>

          <div>
            <HelpTooltipText>
              <strong className={styles.codeExampleLabel}>Connect to the agent</strong>
            </HelpTooltipText>
            <CodeExample code={`ssh coder.${workspaceName}.${agentName}`} />
          </div>
        </Stack>

        <HelpTooltipLinksGroup>
          <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/install">
            Install Coder CLI
          </HelpTooltipLink>
          <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/ides/configuring-web-ides">
            Configuring Web IDEs
          </HelpTooltipLink>
          <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/ides#ssh-configuration">
            SSH configuration
          </HelpTooltipLink>
        </HelpTooltipLinksGroup>
      </Popover>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(3)}px`,
    width: theme.spacing(38),
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.25),
  },

  codeExamples: {
    marginTop: theme.spacing(1.5),
  },

  codeExampleLabel: {
    fontSize: 12,
  },

  textHelper: {
    fontWeight: 400,
  },
}))
