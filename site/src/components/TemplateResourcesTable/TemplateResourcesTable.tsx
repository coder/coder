import { AgentRowPreview } from "components/Resources/AgentRowPreview";
import { Resources } from "components/Resources/Resources";
import { FC } from "react";
import { WorkspaceResource } from "api/typesGenerated";

export interface TemplateResourcesProps {
  resources: WorkspaceResource[];
}

export const TemplateResourcesTable: FC<
  React.PropsWithChildren<TemplateResourcesProps>
> = ({ resources }) => {
  return (
    <Resources
      resources={resources}
      agentRow={(agent, count) => (
        // Align values if there are more than one row
        // When it is only one row, it is better to have it "flex" and not hard aligned
        <AgentRowPreview key={agent.id} agent={agent} alignValues={count > 1} />
      )}
    />
  );
};
