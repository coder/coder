import type { FC } from "react";
import { Link } from "react-router";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	type AdminSettingsPermissions,
	canViewAdminSettings,
	getAdminSettingsItems,
} from "./adminSettings";

type DeploymentDropdownProps = AdminSettingsPermissions;

export const DeploymentDropdown: FC<DeploymentDropdownProps> = (
	permissions,
) => {
	if (!canViewAdminSettings(permissions)) {
		return null;
	}

	const items = getAdminSettingsItems(permissions);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" size="lg">
					Admin settings
					<ChevronDownIcon className="text-content-primary" />
				</Button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="w-[180px] min-w-auto">
				<nav>
					{items.map((item) => (
						<DropdownMenuItem key={item.to} asChild>
							<Link to={item.to}>{item.label}</Link>
						</DropdownMenuItem>
					))}
				</nav>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
