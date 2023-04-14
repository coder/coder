import Popover from "@material-ui/core/Popover"
import CircularProgress from "@material-ui/core/CircularProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { watchAgentMetadata } from "api/api"
import { WorkspaceAgent, WorkspaceAgentMetadata } from "api/typesGenerated"
import { CodeExample } from "components/CodeExample/CodeExample"
import { Stack } from "components/Stack/Stack"
import {
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip"
import dayjs from "dayjs"
import {
  createContext,
  FC,
  PropsWithChildren,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react"
import { humanDuration } from "utils/duration"

export const WatchAgentMetadataContext = createContext(watchAgentMetadata)

const MetadataItemValue: FC<
  PropsWithChildren<{ item: WorkspaceAgentMetadata }>
> = ({ item, children }) => {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLDivElement>(null)
  const styles = useStyles()
  return (
    <>
      <div
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        role="presentation"
      >
        {children}
      </div>
      <Popover
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={() => setIsOpen(false)}
        PaperProps={{
          onMouseEnter: () => setIsOpen(true),
          onMouseLeave: () => setIsOpen(false),
        }}
        classes={{ paper: styles.metadataPopover }}
      >
        <HelpTooltipTitle>{item.description.display_name}</HelpTooltipTitle>
        {item.result.value.length > 0 && (
          <>
            <HelpTooltipText>Last result:</HelpTooltipText>
            <HelpTooltipText>
              <CodeExample code={item.result.value} />
            </HelpTooltipText>
          </>
        )}
        {item.result.error.length > 0 && (
          <>
            <HelpTooltipText>Last error:</HelpTooltipText>
            <HelpTooltipText>
              <CodeExample code={item.result.error} />
            </HelpTooltipText>
          </>
        )}
      </Popover>
    </>
  )
}

const MetadataItem: FC<{ item: WorkspaceAgentMetadata }> = ({ item }) => {
  const styles = useStyles()

  const [isOpen, setIsOpen] = useState(false)

  const labelAnchorRef = useRef<HTMLDivElement>(null)

  if (item.result === undefined) {
    throw new Error("Metadata item result is undefined")
  }
  if (item.description === undefined) {
    throw new Error("Metadata item description is undefined")
  }

  const staleThreshold = Math.max(
    item.description.interval + item.description.timeout * 2,
    5,
  )

  const status: "stale" | "valid" | "loading" = (() => {
    const year = dayjs(item.result.collected_at).year()
    if (year <= 1970 || isNaN(year)) {
      return "loading"
    }
    if (item.result.age > staleThreshold) {
      return "stale"
    }
    return "valid"
  })()

  // Stale data is as good as no data. Plus, we want to build confidence in our
  // users that what's shown is real. If times aren't correctly synced this
  // could be buggy. But, how common is that anyways?
  const value =
    status === "stale" || status === "loading" ? (
      <CircularProgress size={12} />
    ) : (
      <div
        className={
          styles.metadataValue +
          " " +
          (item.result.error.length === 0
            ? styles.metadataValueSuccess
            : styles.metadataValueError)
        }
      >
        {item.result.value}
      </div>
    )

  const updatesInSeconds = -(item.description.interval - item.result.age)

  return (
    <>
      <div className={styles.metadata}>
        <div
          className={styles.metadataLabel}
          onMouseEnter={() => setIsOpen(true)}
          role="presentation"
          ref={labelAnchorRef}
        >
          {item.description.display_name}
        </div>
        <MetadataItemValue item={item}>{value}</MetadataItemValue>
      </div>
      <Popover
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
        open={isOpen}
        anchorEl={labelAnchorRef.current}
        onClose={() => setIsOpen(false)}
        PaperProps={{
          onMouseEnter: () => setIsOpen(true),
          onMouseLeave: () => setIsOpen(false),
        }}
        classes={{ paper: styles.metadataPopover }}
      >
        <HelpTooltipTitle>{item.description.display_name}</HelpTooltipTitle>
        {status === "stale" ? (
          <HelpTooltipText>
            This item is now stale because the agent hasn{"'"}t reported a new
            value in {humanDuration(item.result.age, "s")}.
          </HelpTooltipText>
        ) : (
          <></>
        )}
        {status === "valid" ? (
          <HelpTooltipText>
            The agent collected this value {humanDuration(item.result.age, "s")}{" "}
            ago and will update it in{" "}
            {humanDuration(Math.min(updatesInSeconds, 0), "s")}.
          </HelpTooltipText>
        ) : (
          <></>
        )}
        {status === "loading" ? (
          <HelpTooltipText>
            This value is loading for the first time...
          </HelpTooltipText>
        ) : (
          <></>
        )}
        <HelpTooltipText>
          This value is produced by the following script:
        </HelpTooltipText>
        <HelpTooltipText>
          <CodeExample code={item.description.script}></CodeExample>
        </HelpTooltipText>
      </Popover>
    </>
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
    <Stack
      alignItems="flex-start"
      direction="row"
      spacing={5}
      className={styles.metadataStack}
    >
      <div className={styles.metadataHeader}>
        {metadata.map((m) => {
          if (m.description === undefined) {
            throw new Error("Metadata item description is undefined")
          }
          return <MetadataItem key={m.description.key} item={m} />
        })}
      </div>
    </Stack>
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
      <div
        style={{
          marginTop: 16,
          marginBottom: 16,
        }}
      >
        <CircularProgress size={16} />
      </div>
    )
  }

  return <AgentMetadataView metadata={metadata} />
}

// These are more or less copied from
// site/src/components/Resources/ResourceCard.tsx
const useStyles = makeStyles((theme) => ({
  metadataStack: {
    border: `2px dashed ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    width: "100%",
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(2),
  },
  metadataHeader: {
    padding: "8px",
    display: "flex",
    gap: theme.spacing(5),
    rowGap: theme.spacing(3),
  },

  metadata: {
    fontSize: 16,
  },

  metadataLabel: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    fontWeight: "bold",
  },

  metadataValue: {
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
    maxWidth: "16em",
  },

  metadataValueSuccess: {
    color: theme.palette.success.light,
  },
  metadataValueError: {
    color: theme.palette.error.main,
  },

  metadataPopover: {
    marginTop: theme.spacing(0.5),
    padding: theme.spacing(2.5),
    color: theme.palette.text.secondary,
    pointerEvents: "auto",
    maxWidth: "480px",

    "& .MuiButton-root": {
      padding: theme.spacing(1, 2),
      borderRadius: 0,
      border: 0,

      "&:hover": {
        background: theme.palette.action.hover,
      },
    },
  },
}))
