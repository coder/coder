import Button from "@material-ui/core/Button"
import Popover from "@material-ui/core/Popover"
import Typography from "@material-ui/core/Typography"
import { makeStyles } from "@material-ui/core/styles"
import CloudIcon from "@material-ui/icons/CloudOutlined"
import ExpandMoreIcon from "@material-ui/icons/ExpandMore"
import ChevronRightIcon from "@material-ui/icons/ChevronRight"
import { Maybe } from "components/Conditionals/Maybe"
import { useRef, useState } from "react"
import { CodeExample } from "../CodeExample/CodeExample"
import { Stack } from "../Stack/Stack"
import {
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
} from "../Tooltips/HelpTooltip"
import Link from "@material-ui/core/Link"

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
  const [downloadCLIOpen, setDownloadCLIOpen] = useState(false)
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
        <HelpTooltipText>
          Run the following commands to connect with SSH:
        </HelpTooltipText>

        <Stack spacing={0.5} className={styles.codeExamples}>
          <div>
            <HelpTooltipText>
              <strong className={styles.codeExampleLabel}>
                Configure SSH hosts on machine:
              </strong>
            </HelpTooltipText>
            <CodeExample code="coder config-ssh" />
          </div>

          <div>
            <HelpTooltipText>
              <strong className={styles.codeExampleLabel}>
                Connect to the agent:
              </strong>
            </HelpTooltipText>
            <CodeExample code={`ssh coder.${workspaceName}.${agentName}`} />
          </div>
        </Stack>

        <HelpTooltipLinksGroup>
          <Link
            onClick={() => setDownloadCLIOpen(!downloadCLIOpen)}
            className={styles.link}
          >
            {downloadCLIOpen ? (
              <ExpandMoreIcon className={styles.linkIcon} />
            ) : (
              <ChevronRightIcon className={styles.linkIcon} />
            )}
            <Typography
              className={styles.downloadCLIButton}
              onClick={() => setDownloadCLIOpen(!downloadCLIOpen)}
            >
              Download Coder CLI
            </Typography>
          </Link>
          <Maybe condition={downloadCLIOpen}>
            <div className={styles.downloadCLIOptionsDiv}>
              <HelpTooltipLink href="/bin/coder-windows-amd64.exe">
                For Windows (AMD64)
              </HelpTooltipLink>
              <HelpTooltipLink href="/bin/coder-windows-arm64.exe">
                For Windows (ARM)
              </HelpTooltipLink>
              <HelpTooltipLink href="/bin/coder-darwin-amd64">
                For Mac OS (Intel)
              </HelpTooltipLink>
              <HelpTooltipLink href="/bin/coder-darwin-arm64">
                For Mac OS (ARM)
              </HelpTooltipLink>
              <HelpTooltipLink href="/bin/coder-linux-amd64">
                For Linux (AMD64)
              </HelpTooltipLink>
              <HelpTooltipLink href="/bin/coder-linux-arm64">
                For Linux (ARM64)
              </HelpTooltipLink>
              <HelpTooltipLink href="/bin/coder-linux-arm64">
                For Linux (ARMv7)
              </HelpTooltipLink>
            </div>
          </Maybe>
          <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/ides#vs-code-remote">
            Connect via VS Code Remote SSH
          </HelpTooltipLink>
          <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/ides#jetbrains-gateway">
            Connect via JetBrains Gateway
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
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(
      3,
    )}px`,
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

  downloadCLIButton: {
    fontSize: 14,
  },

  downloadCLIOptionsDiv: {
    display: "flex",
    flexDirection: "column",
    gap: 6,
    marginLeft: theme.spacing(1),
  },

  link: {
    display: "flex",
    alignItems: "center",
    cursor: "pointer",
  },

  linkIcon: {
    width: 14,
    height: 14,
    transform: "scale(1.8)",
    marginRight: theme.spacing(1),
  },
}))
