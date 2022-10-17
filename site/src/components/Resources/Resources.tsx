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
  DERPRegion,
  Workspace,
  WorkspaceAgent,
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
import { ResourceAvatar } from "./ResourceAvatar"
import { SensitiveValue } from "./SensitiveValue"
import { AgentLatency } from "./AgentLatency"

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

  const getDisplayLatency = (agent: WorkspaceAgent) => {
    // Find the right latency to display
    const latencyValues = Object.values(agent.latency ?? {})
    const latency =
      latencyValues.find((derp) => derp.preferred) ??
      // Accessing an array index can return undefined as well
      // for some reason TS does not handle that
      (latencyValues[0] as DERPRegion | undefined)

    if (!latency) {
      return undefined
    }

    // Get the color
    let color = theme.palette.success.light
    if (latency.latency_ms >= 150 && latency.latency_ms < 300) {
      color = theme.palette.warning.light
    } else if (latency.latency_ms >= 300) {
      color = theme.palette.error.light
    }

    return {
      ...latency,
      color,
    }
  }

  return (
    <Stack direction="column" spacing={1}>
      {resources.map((resource) => {
        // Type is already displayed on top of the resource name
        const metadataToDisplay =
          resource.metadata?.filter((data) => data.key !== "type") ?? []

        return (
          <div key={resource.id} className={styles.resourceCard}>
            <Stack
              direction="row"
              alignItems="center"
              className={styles.resourceCardHeader}
            >
              <div>
                <ResourceAvatar resource={resource} />
              </div>
              <Stack direction="row" spacing={4}>
                <div className={styles.resourceData}>
                  <div className={styles.resourceDataLabel}>
                    {resource.type}
                  </div>
                  <div>{resource.name}</div>
                </div>

                {metadataToDisplay.map((data) => (
                  <div key={data.key} className={styles.resourceData}>
                    <div className={styles.resourceDataLabel}>{data.key}</div>
                    {data.sensitive ? (
                      <SensitiveValue value={data.value} />
                    ) : (
                      <div>{data.value}</div>
                    )}
                  </div>
                ))}
              </Stack>
            </Stack>

            <div>
              {resource.agents?.map((agent) => {
                const latency = getDisplayLatency(agent)

                return (
                  <Stack
                    key={agent.id}
                    direction="row"
                    alignItems="center"
                    justifyContent="space-between"
                    className={styles.agentRow}
                  >
                    <Stack direction="row" alignItems="baseline">
                      <div role="status" className={styles.agentStatus} />
                      <div>
                        <div className={styles.agentName}>{agent.name}</div>
                        <Stack
                          direction="row"
                          alignItems="baseline"
                          className={styles.agentData}
                          spacing={1}
                        >
                          <span>{agent.operating_system}</span>
                          <span>{agent.version}</span>
                          {latency && <AgentLatency agent={agent} />}
                        </Stack>
                      </div>
                    </Stack>

                    <Stack direction="row" alignItems="center" spacing={0.5}>
                      {canUpdateWorkspace && agent.status === "connected" && (
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
                              username={workspace.owner_name}
                              workspaceName={workspace.name}
                              agentName={agent.name}
                              health={app.health}
                              appSharingLevel={app.sharing_level}
                            />
                          ))}
                        </>
                      )}
                      {canUpdateWorkspace && agent.status === "connecting" && (
                        <>
                          <Skeleton width={80} height={60} />
                          <Skeleton width={120} height={60} />
                        </>
                      )}
                    </Stack>
                  </Stack>
                )
              })}
            </div>
          </div>
        )
      })}

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

  resourceCard: {
    background: theme.palette.background.paper,
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },

  resourceCardHeader: {
    padding: theme.spacing(3, 4),
    borderBottom: `1px solid ${theme.palette.divider}`,
  },

  resourceData: {
    fontSize: 16,
  },

  resourceDataLabel: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },

  agentRow: {
    padding: theme.spacing(3, 4),
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,

    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
    },
  },

  agentStatus: {
    width: theme.spacing(1),
    height: theme.spacing(1),
    backgroundColor: theme.palette.success.light,
    borderRadius: "100%",
  },

  agentName: {
    fontWeight: 600,
  },

  agentData: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },
}))
