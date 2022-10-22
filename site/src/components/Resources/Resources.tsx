import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { FC, useState } from "react"
import {
  BuildInfoResponse,
  Workspace,
  WorkspaceResource,
} from "../../api/typesGenerated"
import { Stack } from "../Stack/Stack"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ResourceCard } from "./ResourceCard"

const countAgents = (resource: WorkspaceResource) => {
  return resource.agents ? resource.agents.length : 0
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
  buildInfo,
}) => {
  const serverVersion = buildInfo?.version || ""
  const styles = useStyles()
  const [shouldDisplayHideResources, setShouldDisplayHideResources] =
    useState(false)
  const displayResources = shouldDisplayHideResources
    ? resources
    : resources
        .filter((resource) => !resource.hide)
        // Display the resources with agents first
        .sort((a, b) => countAgents(b) - countAgents(a))
  const hasHideResources = resources.some((r) => r.hide)

  if (getResourcesError) {
    return <AlertBanner severity="error" error={getResourcesError} />
  }

  return (
    <Stack direction="column" spacing={0}>
      {displayResources.map((resource) => {
        return (
          <ResourceCard
            key={resource.id}
            resource={resource}
            workspace={workspace}
            applicationsHost={applicationsHost}
            showApps={canUpdateWorkspace}
            hideSSHButton={hideSSHButton}
            serverVersion={serverVersion}
          />
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

const useStyles = makeStyles(() => ({
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
