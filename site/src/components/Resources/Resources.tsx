import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import useTheme from "@material-ui/styles/useTheme"
import React from "react"
import { WorkspaceResource } from "../../api/typesGenerated"
import { getDisplayAgentStatus } from "../../util/workspace"
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
  const styles = useStyles()
  const theme: Theme = useTheme()

  return (
    <WorkspaceSection title={Language.resources} contentsProps={{ className: styles.sectionContents }}>
      {getResourcesError ? (
        { getResourcesError }
      ) : (
        <Table className={styles.table}>
          <TableHead>
            <TableHeaderRow>
              <TableCell>{Language.resourceLabel}</TableCell>
              <TableCell />
            </TableHeaderRow>
          </TableHead>
          <TableBody>
            {resources?.map((resource) => (
              <TableRow key={resource.id}>
                <TableCell>{resource.name}</TableCell>
                <TableCell className={styles.cellWithTable}>
                  {resource.agents && (
                    <Table>
                      <TableHead>
                        <TableHeaderRow>
                          <TableCell width="50%">{Language.agentLabel}</TableCell>
                          <TableCell width="50%">{Language.statusLabel}</TableCell>
                        </TableHeaderRow>
                      </TableHead>
                      <TableBody>
                        {resource.agents.map((agent) => (
                          <TableRow key={`${resource.id}-${agent.id}`}>
                            <TableCell>
                              <span style={{ color: theme.palette.text.secondary }}>{agent.name}</span>
                            </TableCell>
                            <TableCell>
                              <span style={{ color: getDisplayAgentStatus(theme, agent).color }}>
                                {getDisplayAgentStatus(theme, agent).status}
                              </span>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </WorkspaceSection>
  )
}

const useStyles = makeStyles(() => ({
  sectionContents: {
    margin: 0,
  },

  table: {
    border: 0,
  },

  cellWithTable: {
    padding: 0,

    "&:last-child": {
      padding: 0,
    },

    "& table": {
      borderTop: 0,
      borderBottom: 0,

      "& tr:last-child td": {
        borderBottom: 0,
      },
    },
  },
}))
