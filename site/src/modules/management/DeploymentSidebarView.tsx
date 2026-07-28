import {
	ArrowUpRightIcon,
	HardDriveIcon,
	SettingsIcon,
	UserLockIcon,
} from "lucide-react";
import type { FC } from "react";
import type { BuildInfoResponse, Experiment } from "#/api/typesGenerated";
import { Sidebar, SidebarGroup, SidebarNavItem } from "#/components/Sidebar";
import type { Permissions } from "#/modules/permissions";
import { getPrereleaseFlag } from "#/utils/buildInfo";

interface DeploymentSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
	showOrganizations: boolean;
	hasPremiumLicense: boolean;
	experiments: Experiment[];
	buildInfo: BuildInfoResponse;
}

/**
 * Displays navigation for deployment settings grouped into General,
 * Infrastructure, and Authentication sections.
 */
export const DeploymentSidebarView: FC<DeploymentSidebarViewProps> = ({
	permissions,
	showOrganizations,
	hasPremiumLicense,
	experiments,
	buildInfo,
}) => {
	const showOAuth2Apps =
		permissions.viewDeploymentConfig &&
		(experiments.includes("oauth2") ||
			getPrereleaseFlag(buildInfo) === "devel");

	const showGeneral =
		permissions.viewDeploymentConfig ||
		permissions.viewAllLicenses ||
		permissions.editDeploymentConfig ||
		permissions.viewAllUsers ||
		permissions.viewAnyGroup ||
		permissions.viewNotificationTemplate ||
		!hasPremiumLicense;

	const showInfrastructure =
		permissions.viewDeploymentConfig || permissions.readWorkspaceProxies;

	const showAuthentication =
		permissions.viewDeploymentConfig ||
		permissions.viewOrganizationIDPSyncSettings;

	const generalHref = (() => {
		if (permissions.viewDeploymentConfig) {
			return "/deployment/overview";
		}
		if (permissions.viewAllLicenses) {
			return "/deployment/licenses";
		}
		if (permissions.editDeploymentConfig) {
			return "/deployment/appearance";
		}
		if (permissions.viewAllUsers) {
			return "/deployment/users";
		}
		if (permissions.viewAnyGroup) {
			return "/deployment/groups";
		}
		if (permissions.viewNotificationTemplate) {
			return "/deployment/notifications";
		}
		return "/deployment/premium";
	})();

	const infrastructureHref = permissions.viewDeploymentConfig
		? "/deployment/security"
		: "/deployment/workspace-proxies";

	const authenticationHref = permissions.viewDeploymentConfig
		? "/deployment/userauth"
		: "/deployment/idp-org-sync";

	return (
		<Sidebar>
			<div className="flex flex-col gap-1">
				{showGeneral && (
					<SidebarGroup icon={SettingsIcon} label="General" href={generalHref}>
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/deployment/overview">
								Overview
							</SidebarNavItem>
						)}
						{permissions.viewAllLicenses && (
							<SidebarNavItem href="/deployment/licenses">
								Licenses
							</SidebarNavItem>
						)}
						{permissions.editDeploymentConfig && (
							<SidebarNavItem href="/deployment/appearance">
								Appearance
							</SidebarNavItem>
						)}
						{permissions.viewAllUsers && (
							<SidebarNavItem href="/deployment/users">Users</SidebarNavItem>
						)}
						{permissions.viewAnyGroup && (
							<SidebarNavItem href="/deployment/groups">
								<div className="flex flex-row items-center gap-1">
									Groups {showOrganizations && <ArrowUpRightIcon size={16} />}
								</div>
							</SidebarNavItem>
						)}
						{permissions.viewNotificationTemplate && (
							<SidebarNavItem href="/deployment/notifications">
								Notifications
							</SidebarNavItem>
						)}
						{!hasPremiumLicense && (
							<SidebarNavItem href="/deployment/premium">
								Premium
							</SidebarNavItem>
						)}
					</SidebarGroup>
				)}

				{showInfrastructure && (
					<SidebarGroup
						icon={HardDriveIcon}
						label="Infrastructure"
						href={infrastructureHref}
					>
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/deployment/security">
								Security
							</SidebarNavItem>
						)}
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/deployment/observability">
								Observability
							</SidebarNavItem>
						)}
						{permissions.readWorkspaceProxies && (
							<SidebarNavItem href="/deployment/workspace-proxies">
								Workspace proxies
							</SidebarNavItem>
						)}
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/deployment/network">
								Network
							</SidebarNavItem>
						)}
					</SidebarGroup>
				)}

				{showAuthentication && (
					<SidebarGroup
						icon={UserLockIcon}
						label="Authentication"
						href={authenticationHref}
					>
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/deployment/userauth">
								User authentication
							</SidebarNavItem>
						)}
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/deployment/external-auth">
								External authentication
							</SidebarNavItem>
						)}
						{showOAuth2Apps && (
							<SidebarNavItem href="/deployment/oauth2-provider/apps">
								OAuth2 applications
							</SidebarNavItem>
						)}
						{permissions.viewOrganizationIDPSyncSettings && (
							<SidebarNavItem href="/deployment/idp-org-sync">
								IdP organization sync
							</SidebarNavItem>
						)}
					</SidebarGroup>
				)}
			</div>
		</Sidebar>
	);
};
