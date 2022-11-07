import PublicOutlinedIcon from "@material-ui/icons/PublicOutlined"
import LockOutlinedIcon from "@material-ui/icons/LockOutlined"
import GroupOutlinedIcon from "@material-ui/icons/GroupOutlined"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import Tooltip from "@material-ui/core/Tooltip"
import { useTranslation } from "react-i18next"

export interface ShareIconProps {
  app: TypesGen.WorkspaceApp
}

export const ShareIcon: FC<ShareIconProps> = ({ app }) => {
  const { t } = useTranslation("agent")

  let shareIcon = <LockOutlinedIcon />
  let shareTooltip = t("shareTooltip.private")
  if (app.sharing_level === "authenticated") {
    shareIcon = <GroupOutlinedIcon />
    shareTooltip = t("shareTooltip.authenticated")
  }
  if (app.sharing_level === "public") {
    shareIcon = <PublicOutlinedIcon />
    shareTooltip = t("shareTooltip.public")
  }

  return <Tooltip title={shareTooltip}>{shareIcon}</Tooltip>
}
