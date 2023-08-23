import Box, { BoxProps } from "@mui/material/Box"
import Popover from "@mui/material/Popover"
import Skeleton from "@mui/material/Skeleton"
import Tooltip from "@mui/material/Tooltip"
import makeStyles from "@mui/styles/makeStyles"
import { watchAgentMetadata } from "api/api"
import {
  WorkspaceAgent,
  WorkspaceAgentMetadata,
  WorkspaceAgentMetadataResult,
} from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import dayjs from "dayjs"
import {
  FC,
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react"
import { colors } from "theme/colors"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import { combineClasses } from "utils/combineClasses"
import * as XTerm from "xterm"
import { FitAddon } from "xterm-addon-fit"
import { WebglAddon } from "xterm-addon-webgl"
import { Unicode11Addon } from "xterm-addon-unicode11"

import "xterm/css/xterm.css"

type ItemStatus = "stale" | "valid" | "loading"

export const WatchAgentMetadataContext = createContext(watchAgentMetadata)

const MetadataTerminalPopover: FC<{
  id: string
  result: WorkspaceAgentMetadataResult
}> = ({ id, result }) => {
  const styles = useStyles()

  const viewTermRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)

  const [xtermRef, setXtermRef] = useState<HTMLDivElement | null>(null)
  const [terminal, setTerminal] = useState<XTerm.Terminal | null>(null)
  const [fitAddon, setFitAddon] = useState<FitAddon | null>(null)

  const writeTerminal = () => {
    if (!terminal || !fitAddon) {
      return
    }

    // We write the clearCode with the new value to avoid a flash of blankness
    // when the result value updates.
    const clearCode = "\x1B[2J\x1B[H"
    terminal.write(clearCode + result.value, () => {
      fitAddon.fit()
    })
  }

  // Create the terminal.
  // Largely taken from TerminalPage.
  useEffect(() => {
    if (!xtermRef) {
      return
    }
    const terminal = new XTerm.Terminal({
      allowTransparency: true,
      allowProposedApi: true,
      disableStdin: true,
      fontFamily: MONOSPACE_FONT_FAMILY,
      fontSize: 16,
      theme: {
        background: colors.gray[16],
      },
    })
    terminal.loadAddon(new WebglAddon())
    terminal.loadAddon(new FitAddon())

    // This addon fixes multi-width codepoint rendering such as
    // 🟢.
    terminal.loadAddon(new Unicode11Addon())
    terminal.unicode.activeVersion = "11"

    const fitAddon = new FitAddon()
    setTerminal(terminal)
    setFitAddon(fitAddon)
    terminal.open(xtermRef)
    writeTerminal()

    const resizeInterval = setInterval(() => {
      window.dispatchEvent(new Event("resize"))
    }, 100)

    return () => {
      clearInterval(resizeInterval)
      terminal.dispose()
    }
  }, [xtermRef, open])

  useEffect(() => {
    writeTerminal()
  }, [xtermRef, open, result])

  return (
    <>
      <div
        className={styles.viewTerminal}
        ref={viewTermRef}
        onMouseOver={() => {
          setOpen(true)
        }}
      >
        View Terminal
      </div>

      <Popover
        id={id}
        open={open}
        onClose={() => setOpen(false)}
        anchorEl={viewTermRef.current}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
      >
        <div
          className={styles.terminal}
          ref={(el) => {
            setXtermRef(el)
          }}
          data-testid="terminal"
        />
      </Popover>
    </>
  )
}

const MetadataItem: FC<{ item: WorkspaceAgentMetadata }> = ({ item }) => {
  const styles = useStyles()

  if (item.result === undefined) {
    throw new Error("Metadata item result is undefined")
  }
  if (item.description === undefined) {
    throw new Error("Metadata item description is undefined")
  }

  const terminalPrefix = "terminal:"
  const isTerminal = item.description.display_name.startsWith(terminalPrefix)

  const displayName = isTerminal
    ? item.description.display_name.slice(terminalPrefix.length)
    : item.description.display_name

  const staleThreshold = Math.max(
    item.description.interval + item.description.timeout * 2,
    // In case there is intense backpressure, we give a little bit of slack.
    5,
  )

  const status: ItemStatus = (() => {
    const year = dayjs(item.result.collected_at).year()
    if (year <= 1970 || isNaN(year)) {
      return "loading"
    }
    // There is a special circumstance for metadata with `interval: 0`. It is
    // expected that they run once and never again, so never display them as
    // stale.
    if (item.result.age > staleThreshold && item.description.interval > 0) {
      return "stale"
    }
    return "valid"
  })()

  // Stale data is as good as no data. Plus, we want to build confidence in our
  // users that what's shown is real. If times aren't correctly synced this
  // could be buggy. But, how common is that anyways?
  const value =
    status === "loading" ? (
      <Skeleton
        width={65}
        height={12}
        variant="text"
        className={styles.skeleton}
      />
    ) : status === "stale" ? (
      <Tooltip title="This data is stale and no longer up to date">
        <StaticWidth
          className={combineClasses([
            styles.metadataValue,
            styles.metadataStale,
          ])}
        >
          {item.result.value}
        </StaticWidth>
      </Tooltip>
    ) : (
      <StaticWidth
        className={combineClasses([
          styles.metadataValue,
          item.result.error.length === 0
            ? styles.metadataValueSuccess
            : styles.metadataValueError,
        ])}
      >
        {item.result.value}
      </StaticWidth>
    )

  return (
    <div className={styles.metadata}>
      <div className={styles.metadataLabel}>{displayName}</div>
      {isTerminal ? (
        <MetadataTerminalPopover
          id={`metadata-terminal-${item.description.key}`}
          result={item.result}
        />
      ) : (
        <Box>{value}</Box>
      )}
    </div>
  )
}

export interface AgentMetadataViewProps {
  metadata: WorkspaceAgentMetadata[]
}

export const AgentMetadataView: FC<AgentMetadataViewProps> = ({ metadata }) => {
  const styles = useStyles()
  if (metadata.length === 0) {
    return <></>
  }

  return (
    <div className={styles.root}>
      <Stack alignItems="baseline" direction="row" spacing={6}>
        {metadata.map((m) => {
          if (m.description === undefined) {
            throw new Error("Metadata item description is undefined")
          }
          return <MetadataItem key={m.description.key} item={m} />
        })}
      </Stack>
    </div>
  )
}

export const AgentMetadata: FC<{
  agent: WorkspaceAgent
  storybookMetadata?: WorkspaceAgentMetadata[]
}> = ({ agent, storybookMetadata }) => {
  const [metadata, setMetadata] = useState<
    WorkspaceAgentMetadata[] | undefined
  >(undefined)
  const watchAgentMetadata = useContext(WatchAgentMetadataContext)
  const styles = useStyles()

  useEffect(() => {
    if (storybookMetadata !== undefined) {
      setMetadata(storybookMetadata)
      return
    }

    let timeout: NodeJS.Timeout | undefined = undefined

    const connect = (): (() => void) => {
      const source = watchAgentMetadata(agent.id)

      source.onerror = (e) => {
        console.error("received error in watch stream", e)
        setMetadata(undefined)
        source.close()

        timeout = setTimeout(() => {
          connect()
        }, 3000)
      }

      source.addEventListener("data", (e) => {
        const data = JSON.parse(e.data)
        setMetadata(data)
      })
      return () => {
        if (timeout !== undefined) {
          clearTimeout(timeout)
        }
        source.close()
      }
    }
    return connect()
  }, [agent.id, watchAgentMetadata, storybookMetadata])

  if (metadata === undefined) {
    return (
      <div className={styles.root}>
        <AgentMetadataSkeleton />
      </div>
    )
  }

  return <AgentMetadataView metadata={metadata} />
}

export const AgentMetadataSkeleton: FC = () => {
  const styles = useStyles()

  return (
    <Stack alignItems="baseline" direction="row" spacing={6}>
      <div className={styles.metadata}>
        <Skeleton width={40} height={12} variant="text" />
        <Skeleton width={65} height={14} variant="text" />
      </div>

      <div className={styles.metadata}>
        <Skeleton width={40} height={12} variant="text" />
        <Skeleton width={65} height={14} variant="text" />
      </div>

      <div className={styles.metadata}>
        <Skeleton width={40} height={12} variant="text" />
        <Skeleton width={65} height={14} variant="text" />
      </div>
    </Stack>
  )
}

const StaticWidth = (props: BoxProps) => {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    // Ignore this in storybook
    if (!ref.current || process.env.STORYBOOK === "true") {
      return
    }

    const currentWidth = ref.current.getBoundingClientRect().width
    ref.current.style.width = "auto"
    const autoWidth = ref.current.getBoundingClientRect().width
    ref.current.style.width =
      autoWidth > currentWidth ? `${autoWidth}px` : `${currentWidth}px`
  }, [props.children])

  return <Box {...props} ref={ref} />
}

// These are more or less copied from
// site/src/components/Resources/ResourceCard.tsx
const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(2.5, 4),
    borderTop: `1px solid ${theme.palette.divider}`,
    background: theme.palette.background.paper,
    overflowX: "auto",
    scrollPadding: theme.spacing(0, 4),
  },

  viewTerminal: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    display: "inline-block",
    textDecoration: "underline",
    fontWeight: 600,
    margin: 0,
    fontSize: 14,
    borderRadius: 4,
    color: theme.palette.text.primary,
  },

  terminal: {
    width: "80ch",
    overflow: "auto",
    backgroundColor: theme.palette.background.paper,
    // flex: 1,
    padding: theme.spacing(1),
    // These styles attempt to mimic the VS Code scrollbar.
    "& .xterm": {
      padding: 4,
      width: "100vw",
      height: "40vh",
    },
    "& .xterm-viewport": {
      // This is required to force full-width on the terminal.
      // Otherwise there's a small white bar to the right of the scrollbar.
      width: "auto !important",
    },
    "& .xterm-viewport::-webkit-scrollbar": {
      width: "10px",
    },
    "& .xterm-viewport::-webkit-scrollbar-track": {
      backgroundColor: "inherit",
    },
    "& .xterm-viewport::-webkit-scrollbar-thumb": {
      minHeight: 20,
      backgroundColor: "rgba(255, 255, 255, 0.18)",
    },
  },

  popover: {
    padding: 0,
    width: theme.spacing(38),
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },

  metadata: {
    fontSize: 12,
    lineHeight: "normal",
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(0.5),
    overflow: "visible",

    // Because of scrolling
    "&:last-child": {
      paddingRight: theme.spacing(4),
    },
  },

  metadataLabel: {
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    fontWeight: 500,
  },

  metadataValue: {
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    maxWidth: "16em",
    fontSize: 14,
  },

  metadataValueSuccess: {
    color: theme.palette.success.light,
  },

  metadataValueError: {
    color: theme.palette.error.main,
  },

  metadataStale: {
    color: theme.palette.text.disabled,
    cursor: "pointer",
  },

  skeleton: {
    marginTop: theme.spacing(0.5),
  },

  inlineCommand: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    display: "inline-block",
    fontWeight: 600,
    margin: 0,
    borderRadius: 4,
    color: theme.palette.text.primary,
  },
}))
