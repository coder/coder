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
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/deployment">Deployment</Link>
				</DropdownMenuItem>
			)}
			{permissions.canViewOrganizations && (
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/organizations">Organizations</Link>
				</DropdownMenuItem>
			)}
			{permissions.canViewAISettings && (
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/ai/settings">AI</Link>
				</DropdownMenuItem>
			)}
			{permissions.canViewAuditLog && (
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/audit">Audit logs</Link>
				</DropdownMenuItem>
			)}
			{permissions.canViewConnectionLog && (
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/connectionlog">Connection logs</Link>
				</DropdownMenuItem>
			)}
			{permissions.canViewAIBridge && (
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/ai-gateway/sessions">AI sessions</Link>
				</DropdownMenuItem>
			)}
			{permissions.canViewHealth && (
				<DropdownMenuItem asChild className={itemClassName}>
					<Link to="/health">Healthcheck</Link>
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
