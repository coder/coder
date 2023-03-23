import CircularProgress from "@material-ui/core/CircularProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { watchAgentMetadata } from "api/api"
import { WorkspaceAgent, WorkspaceAgentMetadata } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { createContext, FC, useContext, useEffect, useState } from "react"

export const WatchAgentMetadataContext = createContext(watchAgentMetadata)

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

  // Stale data is as good as no data. Plus, we want to build confidence in our
  // users that what's shown is real. If times aren't correctly synced this
  // could be buggy. But, how common is that anyways?
  const value =
    item.result.age < staleThreshold ? (
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
    ) : (
      <CircularProgress size={12} />
    )

  return (
    <div className={styles.metadata}>
      <div className={styles.metadataLabel}>
        {item.description.display_name}
      </div>
      {value}
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
    <Stack alignItems="flex-start" direction="row" spacing={5}>
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
}> = ({ agent }) => {
  const [metadata, setMetadata] = useState<
    WorkspaceAgentMetadata[] | undefined
  >(undefined)

  const watchAgentMetadata = useContext(WatchAgentMetadataContext)

  useEffect(() => {
    const source = watchAgentMetadata(agent.id)

    source.onerror = (e) => {
      console.error("received error in watch stream", e)
    }
    source.addEventListener("data", (e) => {
      const data = JSON.parse(e.data)
      setMetadata(data)
    })
    return () => {
      source.close()
    }
  }, [agent.id, watchAgentMetadata])

  if (metadata === undefined) {
    return <CircularProgress size={16} />
  }

  return <AgentMetadataView metadata={metadata} />
}

// These are more or less copied from
// site/src/components/Resources/ResourceCard.tsx
const useStyles = makeStyles((theme) => ({
  metadataHeader: {
    display: "grid",
    gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
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
  },

  metadataValueSuccess: {
    color: theme.palette.success.light,
  },
  metadataValueError: {
    color: theme.palette.error.main,
  },
}))
