import Avatar from "@material-ui/core/Avatar"
import { makeStyles } from "@material-ui/core/styles"
import FolderIcon from "@material-ui/icons/FolderOutlined"
import ImageIcon from "@material-ui/icons/ImageOutlined"
import MemoryIcon from "@material-ui/icons/MemoryOutlined"
import WidgetsIcon from "@material-ui/icons/WidgetsOutlined"
import React from "react"

// For this special case, we need to apply a different style because how this
// particular icon has been designed
const AdjustedMemoryIcon: typeof MemoryIcon = ({ style, ...props }) => {
  return <MemoryIcon style={{ ...style, fontSize: 24 }} {...props} />
}

const iconByResource: Record<string, typeof MemoryIcon> = {
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

export type ResourceAvatarProps = { type: string }

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
