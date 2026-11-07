import { Link } from "react-router";
import { DropdownMenuItem } from "#/components/DropdownMenu/DropdownMenu";

/**
 * Permissions that determine which items appear in the Admin settings menu.
 * Shared by the desktop `DeploymentDropdown` and the mobile `MobileMenu` so
 * both surfaces render the same set of items from a single source of truth.
 */
type AdminSettingsItemsProps = {
	itemClassName?: string;
	permissions: AdminSettingsPermissions;
};

export type AdminSettingsPermissions = {
	canViewDeployment?: boolean;
	canViewOrganizations?: boolean;
	canViewAISettings?: boolean;
	canViewAuditLog?: boolean;
	canViewConnectionLog?: boolean;
	canViewAIBridge?: boolean;
	canViewHealth?: boolean;
};

/**
 * Builds the ordered list of Admin settings menu items for the given
 * permissions. Organizations is always available; the rest are gated behind
 * their respective permissions.
 */
export const AdminSettingsItems: React.FC<AdminSettingsItemsProps> = ({
	itemClassName,
	permissions,
}) => {
	return (
		<>
			{permissions.canViewDeployment && (
				<DropdownMenuItem
					render={<Link to="/deployment" />}
					className={itemClassName}
				>
					Deployment
				</DropdownMenuItem>
			)}
			{permissions.canViewOrganizations && (
				<DropdownMenuItem
					render={<Link to="/organizations" />}
					className={itemClassName}
				>
					Organizations
				</DropdownMenuItem>
			)}
			{permissions.canViewAISettings && (
				<DropdownMenuItem
					render={<Link to="/ai/settings" />}
					className={itemClassName}
				>
					AI
				</DropdownMenuItem>
			)}
			{permissions.canViewAuditLog && (
				<DropdownMenuItem
					render={<Link to="/audit" />}
					className={itemClassName}
				>
					Audit logs
				</DropdownMenuItem>
			)}
			{permissions.canViewConnectionLog && (
				<DropdownMenuItem
					render={<Link to="/connectionlog" />}
					className={itemClassName}
				>
					Connection logs
				</DropdownMenuItem>
			)}
			{permissions.canViewAIBridge && (
				<DropdownMenuItem
					render={<Link to="/ai-gateway/sessions" />}
					className={itemClassName}
				>
					AI sessions
				</DropdownMenuItem>
			)}
			{permissions.canViewHealth && (
				<DropdownMenuItem
					render={<Link to="/health" />}
					className={itemClassName}
				>
					Healthcheck
				</DropdownMenuItem>
			)}
		</>
	);
};

/**
 * Whether the user has any permission that should surface the Admin settings
 * menu. Organizations alone does not gate visibility, matching prior behavior.
 */
export const canViewAdminSettings = (
	permissions: AdminSettingsPermissions,
): boolean => Object.values(permissions).some((canView) => canView);
