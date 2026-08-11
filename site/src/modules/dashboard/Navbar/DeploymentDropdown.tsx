import type { FC } from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	AdminSettingsItems,
	type AdminSettingsPermissions,
} from "./AdminSettings";

type AdminSettingsDropdownProps = { permissions: AdminSettingsPermissions };

export const AdminSettingsDropdown: FC<AdminSettingsDropdownProps> = ({
	permissions,
}) => {
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
					<AdminSettingsItems permissions={permissions} />
				</nav>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
