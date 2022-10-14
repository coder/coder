import { makeStyles } from "@material-ui/core/styles"
import { Skeleton } from "@material-ui/lab"
import { PortForwardButton } from "components/PortForwardButton/PortForwardButton"
import { FC } from "react"
import { Workspace, WorkspaceResource } from "../../api/typesGenerated"
import { AppLink } from "../AppLink/AppLink"
import { SSHButton } from "../SSHButton/SSHButton"
import { Stack } from "../Stack/Stack"
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { ResourceAvatar } from "./ResourceAvatar"
import { SensitiveValue } from "./SensitiveValue"
import { AgentLatency } from "./AgentLatency"
import { AgentVersion } from "./AgentVersion"

export interface ResourceCardProps {
  resource: WorkspaceResource
  workspace: Workspace
  applicationsHost: string | undefined
  showApps: boolean
  hideSSHButton: boolean
  serverVersion: string
}

export const ResourceCard: FC<ResourceCardProps> = ({
  resource,
  workspace,
  applicationsHost,
  showApps,
  hideSSHButton,
  serverVersion,
}) => {
  const styles = useStyles()

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
        <div className={styles.resourceHeader}>
          <div className={styles.resourceHeaderLabel}>{resource.type}</div>
          <div>{resource.name}</div>
        </div>
      </Stack>

      <Stack
        direction="row"
        alignItems="baseline"
        wrap="wrap"
        className={styles.resourceMetadata}
      >
        {metadataToDisplay.map((data) => (
          <div key={data.key} className={styles.resourceData}>
            <span className={styles.resourceDataLabel}>{data.key}:</span>
            {data.sensitive ? (
              <SensitiveValue value={data.value} />
            ) : (
              <span>{data.value}</span>
            )}
          </div>
        ))}
      </Stack>

      <div>
        {resource.agents?.map((agent) => {
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
                    <span className={styles.agentOS}>
                      {agent.operating_system}
                    </span>
                    <AgentVersion agent={agent} serverVersion={serverVersion} />
                    <AgentLatency agent={agent} />
                  </Stack>
                </div>
              </Stack>

              <Stack direction="row" alignItems="center" spacing={0.5}>
                {showApps && agent.status === "connected" && (
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
                      />
                    ))}
                  </>
                )}
                {showApps && agent.status === "connecting" && (
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
}

const useStyles = makeStyles((theme) => ({
  resourceCard: {
    background: theme.palette.background.paper,
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
  },

  resourceCardHeader: {
    padding: theme.spacing(3, 4),
    borderBottom: `1px solid ${theme.palette.divider}`,
  },

  resourceMetadata: {
    padding: theme.spacing(2, 4),
    borderBottom: `1px solid ${theme.palette.divider}`,
    gap: theme.spacing(0.5, 2),
  },

  resourceHeader: {
    fontSize: 16,
  },

  resourceHeaderLabel: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },

  resourceData: {
    fontSize: 12,
    flexShrink: 0,
  },

  resourceDataLabel: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    marginRight: theme.spacing(0.75),
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

  agentOS: {
    textTransform: "capitalize",
  },

  agentData: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },
}))
