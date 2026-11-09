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

export const AdminSettingsDropdown: React.FC<AdminSettingsDropdownProps> = ({
	permissions,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger render={<Button variant="outline" size="lg" />}>
				Admin settings
				<ChevronDownIcon className="text-content-primary" />
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="w-[180px]">
				<AdminSettingsItems permissions={permissions} />
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
