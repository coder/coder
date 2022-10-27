import PublicOutlinedIcon from "@material-ui/icons/PublicOutlined"
import LockOutlinedIcon from "@material-ui/icons/LockOutlined"
import GroupOutlinedIcon from "@material-ui/icons/GroupOutlined"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import Tooltip from "@material-ui/core/Tooltip"

export interface ShareIconProps {
  app: TypesGen.WorkspaceApp
}

export const ShareIcon: FC<ShareIconProps> = ({ app }) => {
  let shareIcon = <LockOutlinedIcon />
  let shareTooltip = "Private, only accessible by you"
  if (app.sharing_level === "authenticated") {
    shareIcon = <GroupOutlinedIcon />
    shareTooltip = "Shared with all authenticated users"
  }
  if (app.sharing_level === "public") {
    shareIcon = <PublicOutlinedIcon />
    shareTooltip = "Shared publicly"
  }

  return <Tooltip title={shareTooltip}>{shareIcon}</Tooltip>
}
