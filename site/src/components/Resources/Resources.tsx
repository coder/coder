import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import useTheme from "@material-ui/styles/useTheme"
import { FC } from "react"
import { Workspace, WorkspaceResource } from "../../api/typesGenerated"
import { AvatarData } from "../../components/AvatarData/AvatarData"
import { getDisplayAgentStatus } from "../../util/workspace"
import { AppLink } from "../AppLink/AppLink"
import { SSHButton } from "../SSHButton/SSHButton"
import { Stack } from "../Stack/Stack"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { AgentHelpTooltip } from "../Tooltips/AgentHelpTooltip"
import { ResourcesHelpTooltip } from "../Tooltips/ResourcesHelpTooltip"
import { ResourceAvatar } from "./ResourceAvatar"

const Language = {
  resources: "Resources",
  resourceLabel: "Resource",
  agentsLabel: "Agents",
  agentLabel: "Agent",
}

interface ResourcesProps {
  resources?: WorkspaceResource[]
  getResourcesError?: Error
  workspace: Workspace
  canUpdateWorkspace: boolean
}

export const Resources: FC<React.PropsWithChildren<ResourcesProps>> = ({
  resources,
  getResourcesError,
  workspace,
  canUpdateWorkspace,
}) => {
  const styles = useStyles()
  const theme: Theme = useTheme()

  return (
    <div aria-label={Language.resources} className={styles.wrapper}>
      {getResourcesError ? (
        getResourcesError.message
      ) : (
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
                {canUpdateWorkspace && <TableCell></TableCell>}
              </TableHeaderRow>
            </TableHead>
            <TableBody>
              {resources?.map((resource) => {
                {
                  /* We need to initialize the agents to display the resource */
                }
                const agents = resource.agents ?? [null]
                const resourceName = (
                  <AvatarData
                    avatar={<ResourceAvatar type={resource.type} />}
                    title={resource.name}
                    subtitle={resource.type}
                    highlightTitle
                  />
                )

                return agents.map((agent, agentIndex) => {
                  {
                    /* If there is no agent, just display the resource name */
                  }
                  if (!agent) {
                    return (
                      <TableRow key={`${resource.id}-${agentIndex}`}>
                        <TableCell>{resourceName}</TableCell>
                        <TableCell colSpan={3}></TableCell>
                      </TableRow>
                    )
                  }

                  const agentStatus = getDisplayAgentStatus(theme, agent)
                  return (
                    <TableRow key={`${resource.id}-${agent.id}`}>
                      {/* We only want to display the name in the first row because we are using rowSpan */}
                      {/* The rowspan should be the same than the number of agents */}
                      {agentIndex === 0 && (
                        <TableCell className={styles.resourceNameCell} rowSpan={agents.length}>
                          {resourceName}
                        </TableCell>
                      )}

                      <TableCell className={styles.agentColumn}>
                        {agent.name}
                        <div className={styles.agentInfo}>
                          <span className={styles.operatingSystem}>{agent.operating_system}</span>
                          <span style={{ color: agentStatus.color }} className={styles.status}>
                            {agentStatus.status}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <>
                          {canUpdateWorkspace && agent.status === "connected" && (
                            <div className={styles.accessLinks}>
                              <SSHButton workspaceName={workspace.name} agentName={agent.name} />
                              <TerminalLink
                                workspaceName={workspace.name}
                                agentName={agent.name}
                                userName={workspace.owner_name}
                              />
                              {agent.apps.map((app) => (
                                <AppLink
                                  key={app.name}
                                  appIcon={app.icon}
                                  appName={app.name}
                                  userName={workspace.owner_name}
                                  workspaceName={workspace.name}
                                />
                              ))}
                            </div>
                          )}
                        </>
                      </TableCell>
                    </TableRow>
                  )
                })
              })}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },

  tableContainer: {
    border: 0,
  },

  resourceAvatar: {
    color: "#FFF",
    backgroundColor: "#3B73D8",
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
    paddingLeft: `${theme.spacing(4)}px !important`,
  },

  agentInfo: {
    display: "flex",
    gap: theme.spacing(1.5),
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },

  operatingSystem: {
    display: "block",
    textTransform: "capitalize",
  },

  accessLinks: {
    display: "flex",
    gap: theme.spacing(0.5),
    flexWrap: "wrap",
    justifyContent: "flex-end",
  },

  status: {
    whiteSpace: "nowrap",
  },
}))
