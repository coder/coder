import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type {
	BuildInfoResponse,
	ProvisionerKey,
	ProvisionerKeyDaemons,
} from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Stack } from "components/Stack/Stack";
import { ProvisionerGroup } from "modules/provisioners/ProvisionerGroup";
import type { FC } from "react";
import { docs } from "utils/docs";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { Breadcrumbs, Crumb } from "components/Breadcrumbs/Breadcrumbs";

interface OrganizationProvisionersPageViewProps {
	/** Info about the version of coderd */
	buildInfo?: BuildInfoResponse;

	/** Groups of provisioners, along with their key information */
	provisioners: readonly ProvisionerKeyDaemons[];
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ buildInfo, provisioners }) => {
	const { organization } = useOrganizationSettings();
	const isEmpty = provisioners.every((group) => group.daemons.length === 0);

	const provisionerGroupsCount = provisioners.length;
	const provisionersCount = provisioners.reduce(
		(a, group) => a + group.daemons.length,
		0,
	);

	if (!organization) return null;

	return (
		<div>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<Stack direction="row" spacing={2} alignItems="center">
					<Breadcrumbs>
						<Crumb>Organizations</Crumb>
						<Crumb href={`/organizations/${organization}`}>
							{organization.display_name || organization.name}
						</Crumb>
						<Crumb href={`/organizations/${organization}/groups`} active>
							Groups
						</Crumb>
					</Breadcrumbs>
					<FeatureStageBadge contentType="beta" size="sm" />
				</Stack>
				<Button
					endIcon={<OpenInNewIcon />}
					target="_blank"
					href={docs("/admin/provisioners")}
				>
					Create a provisioner
				</Button>
			</Stack>
			{isEmpty ? (
				<EmptyState
					message="No provisioners"
					description="A provisioner is required before you can create templates and workspaces. You can connect your first provisioner by following our documentation."
					cta={
						<Button
							endIcon={<OpenInNewIcon />}
							target="_blank"
							href={docs("/admin/provisioners")}
						>
							Show me how to create a provisioner
						</Button>
					}
				/>
			) : (
				<div
					css={(theme) => ({
						margin: 0,
						fontSize: 12,
						paddingBottom: 18,
						color: theme.palette.text.secondary,
					})}
				>
					Showing {provisionerGroupsCount} groups and {provisionersCount}{" "}
					provisioners
				</div>
			)}
			<Stack spacing={4.5}>
				{provisioners.map((group) => {
					const type = getGroupType(group.key);

					// We intentionally hide user-authenticated provisioners for now
					// because there are 1. some grouping issues on the backend and 2. we
					// should ideally group them by the user who authenticated them, and
					// not just lump them all together.
					if (type === "userAuth") {
						return null;
					}

					return (
						<ProvisionerGroup
							key={group.key.id}
							buildInfo={buildInfo}
							keyName={group.key.name}
							keyTags={group.key.tags}
							type={type}
							provisioners={group.daemons}
						/>
					);
				})}
			</Stack>
		</div>
	);
};

// Ideally these would be generated and appear in typesGenerated.ts, but that is
// not currently the case. In the meantime, these are taken from verbatim from
// the corresponding codersdk declarations. The names remain unchanged to keep
// usage of these special values "grep-able".
// https://github.com/coder/coder/blob/7c77a3cc832fb35d9da4ca27df163c740f786137/codersdk/provisionerdaemons.go#L291-L295
const ProvisionerKeyIDBuiltIn = "00000000-0000-0000-0000-000000000001";
const ProvisionerKeyIDUserAuth = "00000000-0000-0000-0000-000000000002";
const ProvisionerKeyIDPSK = "00000000-0000-0000-0000-000000000003";

function getGroupType(key: ProvisionerKey) {
	switch (key.id) {
		case ProvisionerKeyIDBuiltIn:
			return "builtin";
		case ProvisionerKeyIDUserAuth:
			return "userAuth";
		case ProvisionerKeyIDPSK:
			return "psk";
		default:
			return "key";
	}
}
