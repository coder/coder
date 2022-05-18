import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { WorkspaceResource } from "../../api/typesGenerated"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

const Language = {
  resources: "Resources",
  resourceLabel: "Resource",
  agentsLabel: "Agents",
  agentLabel: "Agent",
  statusLabel: "Status",
}

interface ResourcesProps {
  resources?: WorkspaceResource[]
  getResourcesError?: Error
}

export const Resources: React.FC<ResourcesProps> = ({ resources, getResourcesError }) => {
  return (
    <WorkspaceSection title={Language.resources}>
      {getResourcesError ? (
        { getResourcesError }
      ) : (
        <Table>
          <TableHead>
            <TableHeaderRow>
              <TableCell size="small">{Language.resourceLabel}</TableCell>
              <TableCell size="small">{Language.agentsLabel}</TableCell>
            </TableHeaderRow>
          </TableHead>
          <TableBody>
            {resources?.map((resource) => (
              <TableRow key={resource.id}>
                <TableCell size="small">{resource.name}</TableCell>
                <TableCell>
                  <Table>
                    <TableHead>
                      <TableHeaderRow>
                        <TableCell size="small">{Language.agentLabel}</TableCell>
                        <TableCell size="small">{Language.statusLabel}</TableCell>
                      </TableHeaderRow>
                    </TableHead>
                    <TableBody>
                      {resource.agents?.map((agent) => (
                        <TableRow key={agent.id}>
                          <TableCell size="small">{agent.name}</TableCell>
                          <TableCell size="small">{agent.status}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </WorkspaceSection>
  )
}
