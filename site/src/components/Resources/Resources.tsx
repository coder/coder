import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { Skeleton } from "@material-ui/lab"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { PortForwardButton } from "components/PortForwardButton/PortForwardButton"
import { FC, useState } from "react"
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
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ResourceAvatar } from "./ResourceAvatar"
import { SensitiveValue } from "./SensitiveValue"
import { AgentLatency } from "./AgentLatency"

const getLatency = (agent: WorkspaceAgent) => {
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

  return latency
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
  hideSSHButton,
  applicationsHost,
}) => {
  const styles = useStyles()
  const [shouldDisplayHideResources, setShouldDisplayHideResources] =
    useState(false)
  const displayResources = shouldDisplayHideResources
    ? resources
    : resources.filter((resource) => !resource.hide)
  const hasHideResources = resources.some((r) => r.hide)

  if (getResourcesError) {
    return <AlertBanner severity="error" error={getResourcesError} />
  }

  return (
    <Stack direction="column" spacing={2}>
      {displayResources.map((resource) => {
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
                const latency = getLatency(agent)

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
