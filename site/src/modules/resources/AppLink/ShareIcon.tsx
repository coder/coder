import Tooltip from "@mui/material/Tooltip";
import type * as TypesGen from "api/typesGenerated";
import {
	Building2Icon,
	GlobeIcon,
	SquareArrowOutUpRightIcon,
	UsersIcon,
} from "lucide-react";

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
				<UsersIcon />
			</Tooltip>
		);
	}
	if (app.sharing_level === "organization") {
		return (
			<Tooltip title="Shared with organization members">
				<Building2Icon />
			</Tooltip>
		);
	}
	if (app.sharing_level === "public") {
		return (
			<Tooltip title="Shared publicly">
				<GlobeIcon />
			</Tooltip>
		);
	}

	return null;
};
