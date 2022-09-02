import Avatar from "@material-ui/core/Avatar"
import { makeStyles } from "@material-ui/core/styles"
import FolderIcon from "@material-ui/icons/FolderOutlined"
import ImageIcon from "@material-ui/icons/ImageOutlined"
import MemoryIcon from "@material-ui/icons/MemoryOutlined"
import WidgetsIcon from "@material-ui/icons/WidgetsOutlined"
import React from "react"
import { WorkspaceResource } from "../../api/typesGenerated"

// For this special case, we need to apply a different style because how this
// particular icon has been designed
const AdjustedMemoryIcon: typeof MemoryIcon = ({ style, ...props }) => {
  return <MemoryIcon style={{ ...style, fontSize: 24 }} {...props} />
}

// NOTE@jsjoeio, @BrunoQuaresma
// These resources (i.e. docker_image, kubernetes_deployment) map to Terraform
// resource types. These are the most used ones and are based on user usage.
// We may want to update from time-to-time.
const iconByResource: Record<WorkspaceResource["type"], typeof MemoryIcon | undefined> = {
  docker_volume: FolderIcon,
  docker_container: AdjustedMemoryIcon,
  docker_image: ImageIcon,
  kubernetes_persistent_volume_claim: FolderIcon,
  kubernetes_pod: AdjustedMemoryIcon,
  google_compute_disk: FolderIcon,
  google_compute_instance: AdjustedMemoryIcon,
  aws_instance: AdjustedMemoryIcon,
  kubernetes_deployment: AdjustedMemoryIcon,
  null_resource: WidgetsIcon,
}

export type ResourceAvatarProps = { type: WorkspaceResource["type"] }

export const ResourceAvatar: React.FC<ResourceAvatarProps> = ({ type }) => {
  const IconComponent = iconByResource[type] ?? WidgetsIcon
  const styles = useStyles()

  return (
    <Avatar className={styles.resourceAvatar}>
      <IconComponent style={{ fontSize: 20 }} />
    </Avatar>
  )
}

const useStyles = makeStyles((theme) => ({
  resourceAvatar: {
    color: theme.palette.info.contrastText,
    backgroundColor: theme.palette.info.main,
  },
}))
