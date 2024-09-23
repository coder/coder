import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import { groupsByOrganization } from "api/queries/groups";
import {
	groupIdpSyncSettings,
	roleIdpSyncSettings,
} from "api/queries/organizations";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Loader } from "components/Loader/Loader";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQueries } from "react-query";
import { useParams } from "react-router-dom";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import { IdpSyncHelpTooltip } from "./IdpSyncHelpTooltip";
import IdpSyncPageView from "./IdpSyncPageView";

export const IdpSyncPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organizations } = useOrganizationSettings();
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

	if (
		groupsQuery.isLoading ||
		groupIdpSyncSettingsQuery.isLoading ||
		roleIdpSyncSettingsQuery.isLoading
	) {
		return <Loader />;
	}

	const error =
		groupIdpSyncSettingsQuery.error ||
		roleIdpSyncSettingsQuery.error ||
		groupsQuery.error;
	if (
		error ||
		!groupIdpSyncSettingsQuery.data ||
		!roleIdpSyncSettingsQuery.data ||
		!groupsQuery.data
	) {
		return <ErrorAlert error={error} />;
	}

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
				</Stack>
			</Stack>

			<IdpSyncPageView
				groupSyncSettings={groupIdpSyncSettingsQuery.data}
				roleSyncSettings={roleIdpSyncSettingsQuery.data}
				groups={groupsQuery.data}
				groupsMap={groupsMap}
				organization={organization}
			/>
		</>
	);
};

export default IdpSyncPage;
