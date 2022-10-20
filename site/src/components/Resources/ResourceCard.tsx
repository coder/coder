import { makeStyles } from "@material-ui/core/styles"
import { Skeleton } from "@material-ui/lab"
import { PortForwardButton } from "components/PortForwardButton/PortForwardButton"
import { FC, useState } from "react"
import { Workspace, WorkspaceResource } from "../../api/typesGenerated"
import { AppLink } from "../AppLink/AppLink"
import { SSHButton } from "../SSHButton/SSHButton"
import { Stack } from "../Stack/Stack"
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { ResourceAvatar } from "./ResourceAvatar"
import { SensitiveValue } from "./SensitiveValue"
import { AgentLatency } from "./AgentLatency"
import { AgentVersion } from "./AgentVersion"
import {
  OpenDropdown,
  CloseDropdown,
} from "components/DropdownArrows/DropdownArrows"
import IconButton from "@material-ui/core/IconButton"
import Tooltip from "@material-ui/core/Tooltip"
import { Maybe } from "components/Conditionals/Maybe"
import { CopyableValue } from "components/CopyableValue/CopyableValue"
import { AgentStatus } from "./AgentStatus"

export interface ResourceCardProps {
  resource: WorkspaceResource
  workspace: Workspace
  applicationsHost: string | undefined
  showApps: boolean
  hideSSHButton?: boolean
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
  const [shouldDisplayAllMetadata, setShouldDisplayAllMetadata] =
    useState(false)
  const styles = useStyles()
  const metadataToDisplay =
    // Type is already displayed in the header
    resource.metadata?.filter((data) => data.key !== "type") ?? []
  const visibleMetadata = shouldDisplayAllMetadata
    ? metadataToDisplay
    : metadataToDisplay.slice(0, 4)

  return (
    <div key={resource.id} className={styles.resourceCard}>
      <Stack
        direction="row"
        alignItems="flex-start"
        className={styles.resourceCardHeader}
        spacing={10}
      >
        <Stack
          direction="row"
          alignItems="center"
          className={styles.resourceCardProfile}
        >
          <div>
            <ResourceAvatar resource={resource} />
          </div>
          <div className={styles.metadata}>
            <div className={styles.metadataLabel}>{resource.type}</div>
            <div className={styles.metadataValue}>{resource.name}</div>
          </div>
        </Stack>

        <Stack alignItems="flex-start" direction="row" spacing={5}>
          <div className={styles.metadataHeader}>
            {visibleMetadata.map((meta) => {
              return (
                <div className={styles.metadata} key={meta.key}>
                  <div className={styles.metadataLabel}>{meta.key}</div>
                  <div className={styles.metadataValue}>
                    {meta.sensitive ? (
                      <SensitiveValue value={meta.value} />
                    ) : (
                      <CopyableValue value={meta.value}>
                        {meta.value}
                      </CopyableValue>
                    )}
                  </div>
                </div>
              )
            })}
          </div>

          <Maybe condition={metadataToDisplay.length > 4}>
            <Tooltip
              title={
                shouldDisplayAllMetadata ? "Hide metadata" : "Show all metadata"
              }
            >
              <IconButton
                onClick={() => {
                  setShouldDisplayAllMetadata((value) => !value)
                }}
              >
                {shouldDisplayAllMetadata ? (
                  <CloseDropdown margin={false} />
                ) : (
                  <OpenDropdown margin={false} />
                )}
              </IconButton>
            </Tooltip>
          </Maybe>
        </Stack>
      </Stack>

      {resource.agents && resource.agents.length > 0 && (
        <div>
          {resource.agents.map((agent) => {
            return (
              <Stack
                key={agent.id}
                direction="row"
                alignItems="center"
                justifyContent="space-between"
                className={styles.agentRow}
              >
                <Stack direction="row" alignItems="baseline">
                  <AgentStatus agent={agent} />
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

                      <Maybe condition={agent.status === "connected"}>
                        <AgentVersion
                          agent={agent}
                          serverVersion={serverVersion}
                        />
                      </Maybe>

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
                          appSharingLevel={app.sharing_level}
                        />
                      ))}
                    </>
                  )}
                  {showApps && agent.status === "connecting" && (
                    <>
                      <Skeleton width={80} height={36} variant="rect" />
                      <Skeleton width={120} height={36} variant="rect" />
                    </>
                  )}
                </Stack>
              </Stack>
            )
          })}
        </div>
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  resourceCard: {
    background: theme.palette.background.paper,
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,

    "&:not(:first-child)": {
      borderTop: 0,
      borderTopLeftRadius: 0,
      borderTopRightRadius: 0,
    },

    "&:not(:last-child)": {
      borderBottomLeftRadius: 0,
      borderBottomRightRadius: 0,
    },
  },

  resourceCardProfile: {
    flexShrink: 0,
    width: "fit-content",
  },

  resourceCardHeader: {
    padding: theme.spacing(3, 4),
    borderBottom: `1px solid ${theme.palette.divider}`,

    "&:last-child": {
      borderBottom: 0,
    },
  },

  metadataHeader: {
    display: "grid",
    gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
    gap: theme.spacing(5),
    rowGap: theme.spacing(3),
  },

  metadata: {
    fontSize: 16,
  },

  metadataLabel: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
  },

  metadataValue: {
    textOverflow: "ellipsis",
    overflow: "hidden",
    whiteSpace: "nowrap",
  },

  agentRow: {
    padding: theme.spacing(3, 4),
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,

    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
    },
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
