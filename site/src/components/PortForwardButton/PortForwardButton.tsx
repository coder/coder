import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import OpenInNewOutlined from "@material-ui/icons/OpenInNewOutlined"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Stack } from "components/Stack/Stack"
import { useRef, useState } from "react"
import { colors } from "theme/colors"
import { CodeExample } from "../CodeExample/CodeExample"
import { HelpTooltipLink, HelpTooltipLinksGroup, HelpTooltipText } from "../Tooltips/HelpTooltip"

export interface PortForwardButtonProps {
  host: string
  username: string
  workspaceName: string
  agentName: string
}

const EnabledView: React.FC<PortForwardButtonProps> = (props) => {
  const { host, workspaceName, agentName, username } = props
  const styles = useStyles()
  const [port, setPort] = useState("3000")
  const { location } = window
  const urlExample = `${location.protocol}//${port}--${agentName}--${workspaceName}--${username}.${host}`

  return (
    <Stack direction="column" spacing={1}>
      <HelpTooltipText>
        Access ports running on the agent with the <strong>port, agent name, workspace name</strong>{" "}
        and <strong>your username</strong> URL schema, as shown below.
      </HelpTooltipText>

      <CodeExample code={urlExample} />

      <HelpTooltipText>Use the form to open applications in a new tab.</HelpTooltipText>

      <Stack direction="row" spacing={1} alignItems="center">
        <TextField
          label="Port"
          type="number"
          value={port}
          className={styles.portField}
          onChange={(e) => {
            setPort(e.currentTarget.value)
          }}
        />
        <Link
          underline="none"
          href={urlExample}
          target="_blank"
          rel="noreferrer"
          className={styles.openUrlButton}
        >
          <Button>Open URL</Button>
        </Link>
      </Stack>

      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/networking/port-forwarding#dashboard">
          Learn more about port forward
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </Stack>
  )
}

const DisabledView: React.FC<PortForwardButtonProps> = () => {
  return (
    <Stack direction="column" spacing={1}>
      <HelpTooltipText>
        <strong>Your deployment does not have port forward enabled.</strong> See the docs for more
        details.
      </HelpTooltipText>

      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/networking/port-forwarding#dashboard">
          Learn more about port forward
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </Stack>
  )
}

export const PortForwardButton: React.FC<PortForwardButtonProps> = (props) => {
  const { host } = props
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles()

  const onClose = () => {
    setIsOpen(false)
  }

  return (
    <>
      <Button
        startIcon={<OpenInNewOutlined />}
        size="small"
        ref={anchorRef}
        onClick={() => {
          setIsOpen(true)
        }}
      >
        Port forward
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
        <ChooseOne>
          <Cond condition={host !== ""}>
            <EnabledView {...props} />
          </Cond>
          <Cond>
            <DisabledView {...props} />
          </Cond>
        </ChooseOne>
      </Popover>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  popoverPaper: {
    padding: `${theme.spacing(2.5)}px ${theme.spacing(3.5)}px ${theme.spacing(3.5)}px`,
    width: theme.spacing(46),
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.25),
  },

  openUrlButton: {
    flexShrink: 0,
  },

  portField: {
    // The default border don't contrast well with the popover
    "& .MuiOutlinedInput-root .MuiOutlinedInput-notchedOutline": {
      borderColor: colors.gray[10],
    },
  },
}))
