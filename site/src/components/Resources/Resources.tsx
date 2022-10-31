import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows"
import { FC, useState } from "react"
import { WorkspaceAgent, WorkspaceResource } from "../../api/typesGenerated"
import { Stack } from "../Stack/Stack"
import { ResourceCard } from "./ResourceCard"

const countAgents = (resource: WorkspaceResource) => {
  return resource.agents ? resource.agents.length : 0
}

interface ResourcesProps {
  resources: WorkspaceResource[]
  agentRow: (agent: WorkspaceAgent, numberOfAgents: number) => JSX.Element
}

export const Resources: FC<React.PropsWithChildren<ResourcesProps>> = ({
  resources,
  agentRow,
}) => {
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

  return (
    <Stack direction="column" spacing={0}>
      {displayResources.map((resource) => (
        <ResourceCard
          key={resource.id}
          resource={resource}
          agentRow={(agent) => agentRow(agent, countAgents(resource))}
        />
      ))}
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
  buttonWrapper: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing(2),
  },

  showMoreButton: {
    borderRadius: 9999,
    width: "100%",
    maxWidth: 260,
  },
}))
