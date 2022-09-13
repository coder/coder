import IconButton from "@material-ui/core/IconButton"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import VisibilityOffOutlined from "@material-ui/icons/VisibilityOffOutlined"
import VisibilityOutlined from "@material-ui/icons/VisibilityOutlined"
import { WorkspaceResource } from "api/typesGenerated"
import { FC, useState } from "react"
import { TableCellData, TableCellDataPrimary } from "../TableCellData/TableCellData"
import { ResourceAvatar } from "./ResourceAvatar"

const Language = {
  showLabel: "Show value",
  hideLabel: "Hide value",
}

const SensitiveValue: React.FC<{ value: string }> = ({ value }) => {
  const [shouldDisplay, setShouldDisplay] = useState(false)
  const styles = useStyles()
  const displayValue = shouldDisplay ? value : "••••••••"
  const buttonLabel = shouldDisplay ? Language.hideLabel : Language.showLabel
  const icon = shouldDisplay ? <VisibilityOffOutlined /> : <VisibilityOutlined />

  return (
    <div className={styles.sensitiveValue}>
      {displayValue}
      <Tooltip title={buttonLabel}>
        <IconButton
          className={styles.button}
          onClick={() => {
            setShouldDisplay((value) => !value)
          }}
          size="small"
          aria-label={buttonLabel}
        >
          {icon}
        </IconButton>
      </Tooltip>
    </div>
  )
}

export interface ResourceAvatarDataProps {
  resource: WorkspaceResource
}

export const ResourceAvatarData: FC<ResourceAvatarDataProps> = ({ resource }) => {
  const styles = useStyles()

  return (
    <div className={styles.root}>
      <div className={styles.avatarWrapper}>
        <ResourceAvatar resource={resource} />
      </div>

      <TableCellData>
        <TableCellDataPrimary highlight>{resource.name}</TableCellDataPrimary>
        <div className={styles.data}>
          {resource.metadata?.map((metadata) => (
            <div key={metadata.key} className={styles.dataRow}>
              <strong>{metadata.key}:</strong>
              {metadata.sensitive ? (
                <SensitiveValue value={metadata.value} />
              ) : (
                <div>{metadata.value}</div>
              )}
            </div>
          ))}
        </div>
      </TableCellData>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    display: "flex",
  },

  avatarWrapper: {
    marginRight: theme.spacing(3),
    paddingTop: theme.spacing(0.5),
  },

  data: {
    color: theme.palette.text.secondary,
    fontSize: 14,
    marginTop: theme.spacing(0.75),
    display: "grid",
    gridAutoFlow: "row",
    whiteSpace: "nowrap",
    gap: theme.spacing(0.75),
  },

  dataRow: {
    display: "flex",
    alignItems: "center",

    "& strong": {
      marginRight: theme.spacing(1),
    },
  },

  sensitiveValue: {
    display: "flex",
    alignItems: "center",
  },

  button: {
    marginLeft: theme.spacing(0.5),
    color: "inherit",

    "& .MuiSvgIcon-root": {
      width: 16,
      height: 16,
    },
  },
}))
