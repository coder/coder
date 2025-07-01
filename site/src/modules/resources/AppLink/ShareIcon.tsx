import BusinessIcon from "@mui/icons-material/Business";
import GroupOutlinedIcon from "@mui/icons-material/GroupOutlined";
import PublicOutlinedIcon from "@mui/icons-material/PublicOutlined";
import Tooltip from "@mui/material/Tooltip";
import type * as TypesGen from "api/typesGenerated";
import { SquareArrowOutUpRightIcon } from "lucide-react";

interface ShareIconProps {
	app: TypesGen.WorkspaceApp;
}

export const ShareIcon = ({ app }: ShareIconProps) => {
	if (app.external) {
		return (
			<Tooltip title="Open external URL">
				<SquareArrowOutUpRightIcon />
			</Tooltip>
		);
	}
	if (app.sharing_level === "authenticated") {
		return (
			<Tooltip title="Shared with all authenticated users">
				<GroupOutlinedIcon />
			</Tooltip>
		);
	}
	if (app.sharing_level === "organization") {
		return (
			<Tooltip title="Shared with organization members">
				<BusinessIcon />
			</Tooltip>
		);
	}
	if (app.sharing_level === "public") {
		return (
			<Tooltip title="Shared publicly">
				<PublicOutlinedIcon />
			</Tooltip>
		);
	}

	return null;
};
