import { AgentRowPreview } from "components/Resources/AgentRowPreview"
import { Resources } from "components/Resources/Resources"
import { FC } from "react"
import { WorkspaceResource } from "../../api/typesGenerated"

export interface TemplateResourcesProps {
  resources: WorkspaceResource[]
}

export const TemplateResourcesTable: FC<
  React.PropsWithChildren<TemplateResourcesProps>
> = ({ resources }) => {
  return (
    <Resources
      resources={resources}
      agentRow={(agent) => <AgentRowPreview key={agent.id} agent={agent} />}
    />
  )
}
