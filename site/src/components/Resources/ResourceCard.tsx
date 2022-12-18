import { makeStyles } from "@material-ui/core/styles"
import { FC, useState } from "react"
import { WorkspaceAgent, WorkspaceResource } from "../../api/typesGenerated"
import { Stack } from "../Stack/Stack"
import { ResourceAvatar } from "./ResourceAvatar"
import { SensitiveValue } from "./SensitiveValue"
import {
  OpenDropdown,
  CloseDropdown,
} from "components/DropdownArrows/DropdownArrows"
import IconButton from "@material-ui/core/IconButton"
import Tooltip from "@material-ui/core/Tooltip"
import { Maybe } from "components/Conditionals/Maybe"
import { CopyableValue } from "components/CopyableValue/CopyableValue"

export interface ResourceCardProps {
  resource: WorkspaceResource
  agentRow: (agent: WorkspaceAgent) => JSX.Element
}

export const ResourceCard: FC<ResourceCardProps> = ({ resource, agentRow }) => {
  const [shouldDisplayAllMetadata, setShouldDisplayAllMetadata] =
    useState(false)
  const styles = useStyles()
  const metadataToDisplay = resource.metadata ?? []
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
            {resource.daily_cost > 0 && (
              <div className={styles.metadata}>
                <div className={styles.metadataLabel}>
                  <b>cost</b>
                </div>
                <div className={styles.metadataValue}>
                  {resource.daily_cost}
                </div>
              </div>
            )}
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
        <div>{resource.agents.map(agentRow)}</div>
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
}))
