import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { WorkspaceResource } from "../../api/typesGenerated"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"

const Language = {
  resourceLabel: "Resource",
  agentLabel: "Agent",
}

interface TemplateResourcesProps {
  resources: WorkspaceResource[]
}

export const TemplateResourcesTable: React.FC<TemplateResourcesProps> = ({ resources }) => {
  const styles = useStyles()

  return (
    <Table className={styles.table}>
      <TableHead>
        <TableHeaderRow>
          <TableCell>{Language.resourceLabel}</TableCell>
          <TableCell className={styles.agentColumn}>{Language.agentLabel}</TableCell>
        </TableHeaderRow>
      </TableHead>
      <TableBody>
        {resources.map((resource) => {
          // We need to initialize the agents to display the resource
          const agents = resource.agents ?? [null]
          return agents.map((agent, agentIndex) => {
            //  If there is no agent, just display the resource name
            if (!agent) {
              return (
                <TableRow>
                  <TableCell className={styles.resourceNameCell}>
                    {resource.name}
                    <span className={styles.resourceType}>{resource.type}</span>
                  </TableCell>
                  <TableCell colSpan={3}></TableCell>
                </TableRow>
              )
            }

            return (
              <TableRow key={`${resource.id}-${agent.id}`}>
                {/* We only want to display the name in the first row because we are using rowSpan */}
                {/* The rowspan should be the same than the number of agents */}
                {agentIndex === 0 && (
                  <TableCell className={styles.resourceNameCell} rowSpan={agents.length}>
                    {resource.name}
                    <span className={styles.resourceType}>{resource.type}</span>
                  </TableCell>
                )}

                <TableCell className={styles.agentColumn}>
                  {agent.name}
                  <span className={styles.operatingSystem}>{agent.operating_system}</span>
                </TableCell>
              </TableRow>
            )
          })
        })}
      </TableBody>
    </Table>
  )
}

const useStyles = makeStyles((theme) => ({
  sectionContents: {
    margin: 0,
  },

  table: {
    border: 0,
  },

  resourceNameCell: {
    borderRight: `1px solid ${theme.palette.divider}`,
  },

  resourceType: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
    display: "block",
  },

  // Adds some left spacing
  agentColumn: {
    paddingLeft: `${theme.spacing(2)}px !important`,
  },

  operatingSystem: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
    display: "block",
    textTransform: "capitalize",
  },
}))
