import { useState } from "react";
import { useQueries } from "react-query";
import { organizationSkills } from "#/api/queries/organizationSkills";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { OrganizationField } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RetryButton } from "../../components/RetryButton";
import { SectionHeader } from "../../components/SectionHeader";
import { SkillsTable } from "../../components/SkillsTable";
import {
	fullSkillAccess,
	organizationSkillsScope,
	readOnlySkillAccess,
	userSkillsScope,
} from "../../utils/skills";

const AgentSettingsSkillsPage: React.FC = () => {
	const { user } = useAuthenticated();
	// The dashboard lists every organization the user can read, but chats
	// only run in organizations the user is a member of.
	const memberOrganizations = useDashboard().organizations.filter(
		(organization) => user.organization_ids.includes(organization.id),
	);
	const organizationSkillQueries = useQueries({
		queries: memberOrganizations.map((organization) =>
			organizationSkills(organization.id),
		),
	});
	const visibleOrganizations = memberOrganizations.flatMap(
		(organization, index) => {
			const query = organizationSkillQueries[index];
			const hasEnabledSkills = Boolean(
				query.data?.some((skill) => skill.enabled),
			);
			return hasEnabledSkills || query.isError || query.isPending
				? [{ organization, query, hasEnabledSkills }]
				: [];
		},
	);
	const [selectedOrganizationId, setSelectedOrganizationId] =
		useState<string>();
	const selected =
		visibleOrganizations.find(
			({ organization }) => organization.id === selectedOrganizationId,
		) ??
		visibleOrganizations.find(({ organization }) => organization.is_default) ??
		visibleOrganizations.at(0);
	const organizationName = selected
		? selected.organization.display_name || selected.organization.name
		: "";

	return (
		<div className="flex flex-col gap-12">
			<SkillsTable
				scope={userSkillsScope()}
				copy={{
					noun: "Personal skill",
					title: "Personal skills",
					description:
						"Reusable instructions your agents can pick when they need specialized guidance. Personal skills hold a single SKILL.md file. For richer skills with supporting files, add them to your repo under `.agents/skills/` or load them from a workspace.",
					emptyDescription:
						"Create a personal skill to save reusable agent guidance for your workflows.",
					editorDescription:
						"Personal skills are available to your agents and stored as a single SKILL.md file with frontmatter. For richer skills with supporting files, add them to your repo under `.agents/skills/` or load them from a workspace.",
					archiveName: "personal-skills.zip",
				}}
				access={fullSkillAccess}
			/>
			{selected && (
				<section className="flex flex-col gap-8">
					<SectionHeader
						label="From your organizations"
						description="Organization skills your agents can use in chats that belong to that organization. Members get the skills shared with them. Site owners, organization admins, and auditors get every enabled skill."
						action={
							<OrganizationField
								id="member-skills-organization"
								className="w-full sm:w-60"
								organizations={visibleOrganizations.map(
									({ organization }) => organization,
								)}
								organization={selected.organization}
								onChange={(organization) =>
									setSelectedOrganizationId(organization.id)
								}
								showLabel={false}
							/>
						}
					/>
					{selected.query.isError && !selected.hasEnabledSkills ? (
						// The table's own observer would refetch a failed list on
						// mount and clear its error, so a failed list renders here.
						<div className="flex flex-col gap-4">
							<SectionHeader level="section" label={organizationName} />
							<ErrorAlert
								error={selected.query.error}
								actions={
									<RetryButton
										size="sm"
										variant="outline"
										aria-label={`Retry loading ${organizationName} skills`}
										isRetrying={selected.query.isFetching}
										onRetry={() => {
											void selected.query.refetch();
										}}
									/>
								}
							/>
						</div>
					) : (
						<SkillsTable
							key={selected.organization.id}
							scope={organizationSkillsScope(selected.organization.id)}
							copy={{
								noun: "Organization skill",
								title: organizationName,
								description:
									"Read-only skills available to you. Ask an organization admin to change them.",
								editorDescription:
									"Read-only organization skill. Ask an organization admin to change it.",
								archiveName: `${selected.organization.name}-skills.zip`,
							}}
							access={readOnlySkillAccess}
							enabledOnly
							headerLevel="section"
						/>
					)}
				</section>
			)}
		</div>
	);
};

export default AgentSettingsSkillsPage;
