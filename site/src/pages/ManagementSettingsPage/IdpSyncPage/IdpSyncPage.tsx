import AddIcon from "@mui/icons-material/AddOutlined";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import { groupsByOrganization } from "api/queries/groups";
import {
	groupIdpSyncSettings,
	organizationsPermissions,
	roleIdpSyncSettings,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router-dom";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import { IdpSyncHelpTooltip } from "./IdpSyncHelpTooltip";
import IdpSyncPageView from "./IdpSyncPageView";

export const IdpSyncPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};

	// feature visibility and permissions to be implemented when integrating with backend
	// const feats = useFeatureVisibility();
	// const { organization: organizationName } = useParams() as {
	// 	organization: string;
	// };
	const { organizations } = useOrganizationSettings();

	const organization = organizations?.find((o) => o.name === organizationName);
	const permissionsQuery = useQuery(
		organizationsPermissions(organizations?.map((o) => o.id)),
	);
	const groupIdpSyncSettingsQuery = useQuery(
		groupIdpSyncSettings(organizationName),
	);

	const groupsQuery = useQuery(groupsByOrganization(organizationName));
	const roleIdpSyncSettingsQuery = useQuery(
		roleIdpSyncSettings(organizationName),
	);

	// const permissions = permissionsQuery.data;

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	if (
		permissionsQuery.isLoading ||
		groupIdpSyncSettingsQuery.isLoading ||
		roleIdpSyncSettingsQuery.isLoading
	) {
		return <Loader />;
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
					description="Group and role sync mappings (configured outside Coder)."
					tooltip={<IdpSyncHelpTooltip />}
					badges={<FeatureStageBadge contentType="beta" size="lg" />}
				/>
				<Stack direction="row" spacing={2}>
					<Button
						startIcon={<LaunchOutlined />}
						component="a"
						href={docs("/admin/auth#group-sync-enterprise")}
						target="_blank"
					>
						Setup IdP Sync
					</Button>
					<Button component={RouterLink} startIcon={<AddIcon />} to="export">
						Export Policy
					</Button>
				</Stack>
			</Stack>

			<IdpSyncPageView
				groupSyncSettings={groupIdpSyncSettingsQuery.data}
				roleSyncSettings={roleIdpSyncSettingsQuery.data}
				groups={groupsQuery.data}
			/>
		</>
	);
};

export default IdpSyncPage;
