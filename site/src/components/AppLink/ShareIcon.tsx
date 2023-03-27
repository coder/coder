import PublicOutlinedIcon from "@material-ui/icons/PublicOutlined"
import GroupOutlinedIcon from "@material-ui/icons/GroupOutlined"
import LaunchOutlinedIcon from "@material-ui/icons/LaunchOutlined"
import * as TypesGen from "../../api/typesGenerated"
import Tooltip from "@material-ui/core/Tooltip"
import { useTranslation } from "react-i18next"

export interface ShareIconProps {
  app: TypesGen.WorkspaceApp
}

export const ShareIcon = ({ app }: ShareIconProps) => {
  const { t } = useTranslation("agent")
  if (app.external) {
    return (
      <Tooltip title={t("shareTooltip.external")}>
        <LaunchOutlinedIcon />
      </Tooltip>
    )
  }
  if (app.sharing_level === "authenticated") {
    return (
      <Tooltip title={t("shareTooltip.authenticated")}>
        <GroupOutlinedIcon />
      </Tooltip>
    )
  }
  if (app.sharing_level === "public") {
    return (
      <Tooltip title={t("shareTooltip.public")}>
        <PublicOutlinedIcon />
      </Tooltip>
    )
  }

  return null
}
