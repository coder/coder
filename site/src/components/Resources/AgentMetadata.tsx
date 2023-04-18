import Popover from "@material-ui/core/Popover"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { watchAgentMetadata } from "api/api"
import { WorkspaceAgent, WorkspaceAgentMetadata } from "api/typesGenerated"
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
import { Skeleton } from "@material-ui/lab"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

type ItemStatus = "stale" | "valid" | "loading"

export const WatchAgentMetadataContext = createContext(watchAgentMetadata)

const MetadataItemValue: FC<
  PropsWithChildren<{ item: WorkspaceAgentMetadata; status: ItemStatus }>
> = ({ item, children, status }) => {
  const [isOpen, setIsOpen] = useState(false)
  const anchorRef = useRef<HTMLDivElement>(null)
  const styles = useStyles()
  const updatesInSeconds = -(item.description.interval - item.result.age)

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
        {item.result.error.length > 0 ? (
          <>
            <div className={styles.metadataPopoverContent}>
              <HelpTooltipTitle>
                {item.description.display_name}
              </HelpTooltipTitle>
              <HelpTooltipText>
                An error happened while executing the command{" "}
                <pre className={styles.inlineCommand}>
                  `{item.description.script}`
                </pre>
              </HelpTooltipText>
            </div>
            <div className={styles.metadataPopoverCode}>
              <pre>{item.result.error}</pre>
            </div>
          </>
        ) : (
          <>
            <div className={styles.metadataPopoverContent}>
              <HelpTooltipTitle>
                {item.description.display_name}
              </HelpTooltipTitle>
              {status === "stale" ? (
                <HelpTooltipText>
                  This item is now stale because the agent hasn{"'"}t reported a
                  new value in {humanDuration(item.result.age, "s")}.
                </HelpTooltipText>
              ) : (
                <></>
              )}
              {status === "valid" ? (
                <HelpTooltipText>
                  The agent collected this value{" "}
                  {humanDuration(item.result.age, "s")} ago and will update it
                  in {humanDuration(Math.min(updatesInSeconds, 0), "s")}.
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
            </div>
            <div className={styles.metadataPopoverCode}>
              <pre>{item.description.script}</pre>
            </div>
          </>
        )}
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

  const staleThreshold = Math.max(
    item.description.interval + item.description.timeout * 2,
    5,
  )

  const status: ItemStatus = (() => {
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
      <Skeleton
        width={65}
        height={12}
        variant="text"
        className={styles.skeleton}
      />
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

  return (
    <div className={styles.metadata}>
      <div className={styles.metadataLabel} role="presentation">
        {item.description.display_name}
      </div>
      <MetadataItemValue item={item} status={status}>
        {value}
      </MetadataItemValue>
    </div>
  )
}

export interface AgentMetadataViewProps {
  metadata: WorkspaceAgentMetadata[]
}

export const AgentMetadataView: FC<AgentMetadataViewProps> = ({ metadata }) => {
  if (metadata.length === 0) {
    return <></>
  }
  return (
    <Stack alignItems="baseline" direction="row" spacing={6}>
      {metadata.map((m) => {
        if (m.description === undefined) {
          throw new Error("Metadata item description is undefined")
        }
        return <MetadataItem key={m.description.key} item={m} />
      })}
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
  const styles = useStyles()
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
      <Skeleton
        width={65}
        height={12}
        variant="text"
        className={styles.skeleton}
      />
    )
  }

  return <AgentMetadataView metadata={metadata} />
}

// These are more or less copied from
// site/src/components/Resources/ResourceCard.tsx
const useStyles = makeStyles((theme) => ({
  metadata: {
    fontSize: 12,
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
  },

  metadataValueSuccess: {
    color: theme.palette.text.primary,
  },

  metadataValueError: {
    color: theme.palette.error.main,
  },

  metadataPopover: {
    marginTop: theme.spacing(0.5),

    color: theme.palette.text.secondary,
    pointerEvents: "auto",
    width: "320px",
    borderRadius: 4,

    "& .MuiButton-root": {
      padding: theme.spacing(1, 2),
      borderRadius: 0,
      border: 0,

      "&:hover": {
        background: theme.palette.action.hover,
      },
    },
  },

  metadataPopoverContent: {
    padding: theme.spacing(2.5),
  },

  metadataPopoverCode: {
    padding: theme.spacing(2.5),
    fontFamily: MONOSPACE_FONT_FAMILY,
    background: theme.palette.background.default,
    color: theme.palette.text.primary,

    "& pre": {
      padding: 0,
      margin: 0,
    },
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
