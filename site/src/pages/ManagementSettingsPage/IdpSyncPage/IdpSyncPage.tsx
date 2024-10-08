import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import { groupsByOrganization } from "api/queries/groups";
import {
	groupIdpSyncSettings,
	roleIdpSyncSettings,
} from "api/queries/organizations";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useManagementSettings } from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQueries } from "react-query";
import { useParams } from "react-router-dom";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { IdpSyncHelpTooltip } from "./IdpSyncHelpTooltip";
import IdpSyncPageView from "./IdpSyncPageView";

export const IdpSyncPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	// IdP sync does not have its own entitlement and is based on templace_rbac
	const { template_rbac: isIdpSyncEnabled } = useFeatureVisibility();
	const { organizations } = useManagementSettings();
	const organization = organizations?.find((o) => o.name === organizationName);

	const [groupIdpSyncSettingsQuery, roleIdpSyncSettingsQuery, groupsQuery] =
		useQueries({
			queries: [
				groupIdpSyncSettings(organizationName),
				roleIdpSyncSettings(organizationName),
				groupsByOrganization(organizationName),
			],
		});

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const error =
		groupIdpSyncSettingsQuery.error ||
		roleIdpSyncSettingsQuery.error ||
		groupsQuery.error;

	const groupsMap = new Map<string, string>();
	if (groupsQuery.data) {
		for (const group of groupsQuery.data) {
			groupsMap.set(group.id, group.display_name || group.name);
		}
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle("IdP Sync")}</title>
			</Helmet>

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title="IdP Sync"
					description="Group and role sync mappings (configured using Coder CLI)."
					tooltip={<IdpSyncHelpTooltip />}
				/>
				<Button
					startIcon={<LaunchOutlined />}
					component="a"
					href={docs("/admin/users/idp-sync")}
					target="_blank"
				>
					Setup IdP Sync
				</Button>
			</Stack>
			<ChooseOne>
				<Cond condition={!isIdpSyncEnabled}>
					<Paywall
						message="IdP Sync"
						description="Configure group and role mappings to manage permissions outside of Coder. You need an Premium license to use this feature."
						documentationLink={docs("/admin/users/idp-sync")}
					/>
				</Cond>
				<Cond>
					<IdpSyncPageView
						groupSyncSettings={groupIdpSyncSettingsQuery.data}
						roleSyncSettings={roleIdpSyncSettingsQuery.data}
						groups={groupsQuery.data}
						groupsMap={groupsMap}
						organization={organization}
						error={error}
					/>
				</Cond>
			</ChooseOne>
		</>
	);
};

export default IdpSyncPage;
