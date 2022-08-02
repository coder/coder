import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { AvatarData } from "components/AvatarData/AvatarData"
import { ResourceAvatar } from "components/Resources/ResourceAvatar"
import { FC } from "react"
import { WorkspaceResource } from "../../api/typesGenerated"
import { Stack } from "../Stack/Stack"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { AgentHelpTooltip } from "../Tooltips/AgentHelpTooltip"
import { ResourcesHelpTooltip } from "../Tooltips/ResourcesHelpTooltip"

export const Language = {
  resourceLabel: "Resource",
  agentLabel: "Agent",
}

export interface TemplateResourcesProps {
  resources: WorkspaceResource[]
}

export const TemplateResourcesTable: FC<React.PropsWithChildren<TemplateResourcesProps>> = ({ resources }) => {
  const styles = useStyles()

  return (
    <TableContainer className={styles.tableContainer}>
      <Table>
        <TableHead>
          <TableHeaderRow>
            <TableCell>
              <Stack direction="row" spacing={0.5} alignItems="center">
                {Language.resourceLabel}
                <ResourcesHelpTooltip />
              </Stack>
            </TableCell>
            <TableCell className={styles.agentColumn}>
              <Stack direction="row" spacing={0.5} alignItems="center">
                {Language.agentLabel}
                <AgentHelpTooltip />
              </Stack>
            </TableCell>
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
                      <AvatarData
                        title={resource.name}
                        subtitle={resource.type}
                        highlightTitle
                        avatar={<ResourceAvatar type={resource.type} />}
                      />
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
                      <AvatarData
                        title={resource.name}
                        subtitle={resource.type}
                        highlightTitle
                        avatar={<ResourceAvatar type={resource.type} />}
                      />
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
    </TableContainer>
  )
}

const useStyles = makeStyles((theme) => ({
  sectionContents: {
    margin: 0,
  },

  tableContainer: {
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
