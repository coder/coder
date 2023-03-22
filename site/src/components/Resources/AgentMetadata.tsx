import CircularProgress from "@material-ui/core/CircularProgress"
import { watchAgentMetadata } from "api/api"
import { WorkspaceAgent, WorkspaceAgentMetadata } from "api/typesGenerated"
import dayjs from "dayjs"
import { FC, useEffect, useState } from "react"

const MetadataItem: FC<{ item: WorkspaceAgentMetadata }> = ({ item }) => {
  if (item.result === undefined) {
    throw new Error("Metadata item result is undefined")
  }
  if (item.description === undefined) {
    throw new Error("Metadata item description is undefined")
  }

  if (dayjs(item.result.collected_at).year() === 0) {
    // Still loading.
    return (
      <div>
        {item.description.display_name}: <CircularProgress size={12} />
      </div>
    )
  }
  return (
    <div>
      {item.description.display_name}: {item.result.value}
    </div>
  )
}

export const AgentMetadata: FC<{ agent: WorkspaceAgent }> = ({ agent }) => {
  const [metadata, setMetadata] = useState<
    WorkspaceAgentMetadata[] | undefined
  >(undefined)

  useEffect(() => {
    const source = watchAgentMetadata(agent.id)
    source.onerror = (e) => {
      console.error(e)
    }
    source.addEventListener("data", (e) => {
      const data = JSON.parse(e.data)
      setMetadata(data)
    })
    return () => {
      source.close()
    }
  }, [agent.id])

  if (metadata === undefined) {
    return <CircularProgress size={16} />
  }
  return (
    <div>
      {metadata.map((m) => {
        if (m.description === undefined) {
          throw new Error("Metadata item description is undefined")
        }
        return <MetadataItem key={m.description.key} item={m} />
      })}
    </div>
  )
}
