import { useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { SkillsTable } from "#/pages/AgentsPage/components/SkillsTable";
import { pageTitle } from "#/utils/page";
import { OrganizationPicker } from "../MCPServersPage/components/OrganizationPicker";
import {
	orgSearchParam,
	selectOrganization,
} from "../MCPServersPage/organizationParam";
import { OrganizationSkillSharingDialog } from "./OrganizationSkillSharingDialog";

const SkillsPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const [searchParams, setSearchParams] = useSearchParams();
	const [sharingSkill, setSharingSkill] = useState<{
		organizationId: string;
		name: string;
	}>();
	const organizationPermissionsQuery = useQuery({
		...organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
		enabled: permissions.viewAnyOrganizationSkills,
	});
	const readableOrganizations = organizations.filter(
		(organization) =>
			organizationPermissionsQuery.data?.[organization.id]
				?.viewOrganizationSkills,
	);
	const requestedOrganizationName = searchParams.get(orgSearchParam);
	const defaultOrganization =
		readableOrganizations.length > 0
			? selectOrganization(readableOrganizations, null)
			: undefined;
	// A requested organization the user cannot read is denied rather than
	// replaced, so a skill is never added to an organization the URL did not
	// name.
	const organization =
		requestedOrganizationName === null
			? defaultOrganization
			: readableOrganizations.find(
					(organization) => organization.name === requestedOrganizationName,
				);
	const organizationPermissions = organization
		? organizationPermissionsQuery.data?.[organization.id]
		: undefined;
	const canEdit = Boolean(
		organizationPermissions?.createOrganizationSkill &&
			organizationPermissions.updateOrganizationSkill &&
			organizationPermissions.deleteOrganizationSkill,
	);

	return (
		<RequirePermission isFeatureVisible={permissions.viewAnyOrganizationSkills}>
			<title>{pageTitle("Skills", "AI Settings")}</title>
			{organizationPermissionsQuery.isLoadingError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !organizationPermissionsQuery.data ? (
				<Loader />
			) : (
				<RequirePermission isFeatureVisible={Boolean(organization)}>
					{organizationPermissionsQuery.isRefetchError && (
						<div className="mb-4">
							<ErrorAlert error={organizationPermissionsQuery.error} />
						</div>
					)}
					{organization && (
						<SkillsTable
							// Reset dialogs and in-flight state when the organization changes.
							key={organization.id}
							owner={{
								type: "organization",
								organizationId: organization.id,
							}}
							copy={{
								noun: "Organization skill",
								title: "Skills",
								description:
									"Reusable instructions that agents in this organization can load when they need specialized guidance. Each skill holds a single SKILL.md file and is shared with everyone in the organization by default.",
								emptyDescription:
									"Add a skill to give agents in this organization reusable guidance.",
								editorDescription:
									"Organization skills are available to agents in this organization and stored as a single SKILL.md file with frontmatter.",
								archiveName: `${organization.name}-skills.zip`,
							}}
							canEdit={canEdit}
							onManagePermissions={
								organizationPermissions?.shareOrganizationSkill
									? (skill) =>
											setSharingSkill({
												organizationId: organization.id,
												name: skill.name,
											})
									: undefined
							}
							toolbar={
								<OrganizationPicker
									id="skills-organization"
									className="w-full sm:w-60"
									organizations={readableOrganizations}
									organization={organization}
									onChange={(org) => {
										setSearchParams((params) => {
											const next = new URLSearchParams(params);
											next.set(orgSearchParam, org.name);
											return next;
										});
									}}
									showLabel={false}
								/>
							}
						/>
					)}
					{sharingSkill && (
						<OrganizationSkillSharingDialog
							organizationId={sharingSkill.organizationId}
							skillName={sharingSkill.name}
							onClose={() => setSharingSkill(undefined)}
						/>
					)}
				</RequirePermission>
			)}
		</RequirePermission>
	);
};

export default SkillsPage;
