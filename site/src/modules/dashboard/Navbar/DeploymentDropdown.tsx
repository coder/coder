import type { FC } from "react";
import { Link } from "react-router";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	type AdminSettingsPermissions,
	canViewAdminSettings,
	getAdminSettingsSections,
} from "./adminSettings";

type DeploymentDropdownProps = AdminSettingsPermissions;

export const DeploymentDropdown: FC<DeploymentDropdownProps> = (
	permissions,
) => {
	if (!canViewAdminSettings(permissions)) {
		return null;
	}

	const sections = getAdminSettingsSections(permissions);

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
					{sections.map((section, index) => (
						<div key={section.label ?? section.items[0].to}>
							{index > 0 && <DropdownMenuSeparator />}
							{section.label && (
								<div className="px-2 py-1.5 text-xs font-medium text-content-secondary">
									{section.label}
								</div>
							)}
							{section.items.map((item) => (
								<DropdownMenuItem key={item.to} asChild>
									<Link to={item.to}>{item.label}</Link>
								</DropdownMenuItem>
							))}
						</div>
					))}
				</nav>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
