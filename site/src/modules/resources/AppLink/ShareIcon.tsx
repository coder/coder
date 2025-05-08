import Tooltip from "@mui/material/Tooltip";
import type * as TypesGen from "api/typesGenerated";
import {
	GroupOutlinedIcon,
	LaunchOutlinedIcon,
	PublicOutlinedIcon,
} from "lucide-react";

export interface ShareIconProps {
	app: TypesGen.WorkspaceApp;
}

export const ShareIcon = ({ app }: ShareIconProps) => {
	if (app.external) {
		return (
			<Tooltip title="Open external URL">
				<LaunchOutlinedIcon />
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
	if (app.sharing_level === "public") {
		return (
			<Tooltip title="Shared publicly">
				<PublicOutlinedIcon />
			</Tooltip>
		);
	}

	return null;
};
