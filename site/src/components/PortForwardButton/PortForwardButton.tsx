import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import OpenInNewOutlined from "@material-ui/icons/OpenInNewOutlined"
import { Stack } from "components/Stack/Stack"
import { useRef, useState } from "react"
import { colors } from "theme/colors"
import { CodeExample } from "../CodeExample/CodeExample"
import { HelpTooltipLink, HelpTooltipLinksGroup, HelpTooltipText } from "../Tooltips/HelpTooltip"

export interface PortForwardButtonProps {
  username: string
  workspaceName: string
  agentName: string
  defaultIsOpen?: boolean
}

export const PortForwardButton: React.FC<React.PropsWithChildren<PortForwardButtonProps>> = ({
  workspaceName,
  agentName,
  username,
  defaultIsOpen = false,
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(defaultIsOpen)
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles()
  const [port, setPort] = useState("3000")
  const { location } = window
  const urlExample =
    process.env.CODER_ENABLE_WILDCARD_APPS === "true"
      ? `${location.protocol}//${port}--${workspaceName}--${agentName}--${username}.${location.host}`
      : `${location.protocol}//${location.host}/@${username}/${workspaceName}/apps/${port}`

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
        <Stack direction="column" spacing={1}>
          <HelpTooltipText>
            You can port forward this resource by typing the{" "}
            <strong>port, workspace name, agent name</strong> and <strong>your username</strong> in
            the URL like the example below
          </HelpTooltipText>

          <CodeExample code={urlExample} />

          <HelpTooltipText>
            Or you can use the following form to open it in a new tab.
          </HelpTooltipText>

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
            <HelpTooltipLink href="https://coder.com/docs/coder-oss/latest/port-forward">
              Port forward
            </HelpTooltipLink>
          </HelpTooltipLinksGroup>
        </Stack>
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
