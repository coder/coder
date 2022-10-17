import Button from "@material-ui/core/Button"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { Skeleton } from "@material-ui/lab"
import useTheme from "@material-ui/styles/useTheme"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { PortForwardButton } from "components/PortForwardButton/PortForwardButton"
import { TableCellDataPrimary } from "components/TableCellData/TableCellData"
import { FC, useState } from "react"
import { getDisplayAgentStatus, getDisplayVersionStatus } from "util/workspace"
import {
  BuildInfoResponse,
  Workspace,
  WorkspaceResource,
} from "../../api/typesGenerated"
import { AppLink } from "../AppLink/AppLink"
import { SSHButton } from "../SSHButton/SSHButton"
import { Stack } from "../Stack/Stack"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { AgentHelpTooltip } from "../Tooltips/AgentHelpTooltip"
import { AgentOutdatedTooltip } from "../Tooltips/AgentOutdatedTooltip"
import { ResourcesHelpTooltip } from "../Tooltips/ResourcesHelpTooltip"
import { ResourceAgentLatency } from "./ResourceAgentLatency"
import { ResourceAvatarData } from "./ResourceAvatarData"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

const Language = {
  resources: "Resources",
  resourceLabel: "Resource",
  agentsLabel: "Agents",
  agentLabel: "Agent",
  statusLabel: "status: ",
  versionLabel: "version: ",
  osLabel: "os: ",
}

interface ResourcesProps {
  resources: WorkspaceResource[]
  getResourcesError?: Error | unknown
  workspace: Workspace
  canUpdateWorkspace: boolean
  buildInfo?: BuildInfoResponse | undefined
  hideSSHButton?: boolean
  applicationsHost?: string
}

export const Resources: FC<React.PropsWithChildren<ResourcesProps>> = ({
  resources,
  getResourcesError,
  workspace,
  canUpdateWorkspace,
  buildInfo,
  hideSSHButton,
  applicationsHost,
}) => {
  const styles = useStyles()
  const theme: Theme = useTheme()
  const serverVersion = buildInfo?.version || ""
  const [shouldDisplayHideResources, setShouldDisplayHideResources] =
    useState(false)
  const displayResources = shouldDisplayHideResources
    ? resources
    : resources.filter((resource) => !resource.hide)
  const hasHideResources = resources.some((r) => r.hide)

  return (
    <Stack direction="column" spacing={1}>
      <div aria-label={Language.resources} className={styles.wrapper}>
        {getResourcesError ? (
          <AlertBanner severity="error" error={getResourcesError} />
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
                {displayResources.map((resource) => {
                  {
                    /* We need to initialize the agents to display the resource */
                  }
                  const agents = resource.agents ?? [null]
                  const resourceName = (
                    <ResourceAvatarData resource={resource} />
                  )

                  return agents.map((agent, agentIndex) => {
                    {
                      /* If there is no agent, just display the resource name */
                    }
                    if (
                      !agent ||
                      workspace.latest_build.transition === "stop"
                    ) {
                      return (
                        <TableRow key={`${resource.id}-${agentIndex}`}>
                          <TableCell>{resourceName}</TableCell>
                          <TableCell colSpan={3}></TableCell>
                        </TableRow>
                      )
                    }
                    const { displayVersion, outdated } =
                      getDisplayVersionStatus(agent.version, serverVersion)
                    const agentStatus = getDisplayAgentStatus(theme, agent)
                    return (
                      <TableRow key={`${resource.id}-${agent.id}`}>
                        {/* We only want to display the name in the first row because we are using rowSpan */}
                        {/* The rowspan should be the same than the number of agents */}
                        {agentIndex === 0 && (
                          <TableCell
                            className={styles.resourceNameCell}
                            rowSpan={agents.length}
                          >
                            {resourceName}
                          </TableCell>
                        )}

                        <TableCell className={styles.agentColumn}>
                          <TableCellDataPrimary highlight>
                            {agent.name}
                          </TableCellDataPrimary>
                          <div className={styles.data}>
                            <div className={styles.dataRow}>
                              <strong>{Language.statusLabel}</strong>
                              <span
                                style={{ color: agentStatus.color }}
                                className={styles.status}
                              >
                                {agentStatus.status}
                              </span>
                            </div>
                            <div className={styles.dataRow}>
                              <strong>{Language.osLabel}</strong>
                              <span className={styles.operatingSystem}>
                                {agent.operating_system}
                              </span>
                            </div>
                            <div className={styles.dataRow}>
                              <strong>{Language.versionLabel}</strong>
                              <span className={styles.agentVersion}>
                                {displayVersion}
                              </span>
                              <AgentOutdatedTooltip outdated={outdated} />
                            </div>
                            <div className={styles.dataRow}>
                              <ResourceAgentLatency latency={agent.latency} />
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className={styles.accessLinks}>
                            {canUpdateWorkspace &&
                              agent.status === "connected" && (
                                <>
                                  {applicationsHost !== undefined && (
                                    <PortForwardButton
                                      host={applicationsHost}
                                      workspaceName={workspace.name}
                                      agentId={agent.id}
                                      agentName={agent.name}
                                      username={workspace.owner_name}
                                    />
                                  )}
                                  {!hideSSHButton && (
                                    <SSHButton
                                      workspaceName={workspace.name}
                                      agentName={agent.name}
                                    />
                                  )}
                                  <TerminalLink
                                    workspaceName={workspace.name}
                                    agentName={agent.name}
                                    userName={workspace.owner_name}
                                  />
                                  {agent.apps.map((app) => (
                                    <AppLink
                                      key={app.name}
                                      appsHost={applicationsHost}
                                      appIcon={app.icon}
                                      appName={app.name}
                                      appCommand={app.command}
                                      appSubdomain={app.subdomain}
                                      appSharingLevel={app.sharing_level}
                                      username={workspace.owner_name}
                                      workspaceName={workspace.name}
                                      agentName={agent.name}
                                      health={app.health}
                                    />
                                  ))}
                                </>
                              )}
                            {canUpdateWorkspace &&
                              agent.status === "connecting" && (
                                <>
                                  <Skeleton width={80} height={60} />
                                  <Skeleton width={120} height={60} />
                                </>
                              )}
                          </div>
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

      {hasHideResources && (
        <div className={styles.buttonWrapper}>
          <Button
            className={styles.showMoreButton}
            variant="outlined"
            size="small"
            onClick={() => setShouldDisplayHideResources((v) => !v)}
          >
            {shouldDisplayHideResources ? (
              <>
                Hide resources <CloseDropdown />
              </>
            ) : (
              <>
                Show hidden resources <OpenDropdown />
              </>
            )}
          </Button>
        </div>
      )}
    </Stack>
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

  operatingSystem: {
    display: "block",
    textTransform: "capitalize",
  },

  agentVersion: {
    display: "block",
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

  data: {
    color: theme.palette.text.secondary,
    fontSize: 14,
    marginTop: theme.spacing(0.75),
    display: "grid",
    gridAutoFlow: "row",
    whiteSpace: "nowrap",
    gap: theme.spacing(0.75),
    height: "fit-content",
  },

  dataRow: {
    display: "flex",
    alignItems: "center",

    "& strong": {
      marginRight: theme.spacing(1),
    },
  },

  buttonWrapper: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },

  showMoreButton: {
    borderRadius: 9999,
    width: "100%",
    maxWidth: 260,
  },
}))
