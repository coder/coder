import { useState } from "react";
import { type UseQueryResult, useQuery } from "react-query";
import { organizationSkillPreferences } from "#/api/queries/organizationSkills";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { OrganizationField } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { RetryButton } from "#/components/RetryButton/RetryButton";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getMemberOrganizations,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { OrganizationSkillPreferenceSwitch } from "../../components/OrganizationSkillPreferenceSwitch";
import { OrganizationSkillsMasterSwitch } from "../../components/OrganizationSkillsMasterSwitch";
import { SectionHeader } from "../../components/SectionHeader";
import { SkillsTable } from "../../components/SkillsTable";
import {
	fullSkillAccess,
	organizationSkillsScope,
	readOnlySkillAccess,
	userSkillsScope,
} from "../../utils/skills";

type QueryErrorAlertProps = {
	query: Pick<UseQueryResult<unknown>, "error" | "isFetching" | "refetch">;
	retryLabel: string;
};

const QueryErrorAlert: React.FC<QueryErrorAlertProps> = ({
	query,
	retryLabel,
}) => (
	<ErrorAlert
		error={query.error}
		actions={
			<RetryButton
				size="sm"
				variant="outline"
				aria-label={retryLabel}
				isRetrying={query.isFetching}
				onRetry={() => {
					void query.refetch();
				}}
			/>
		}
	/>
);

const AgentSettingsSkillsPage: React.FC = () => {
	const { user } = useAuthenticated();
	const memberOrganizations = getMemberOrganizations(
		useDashboard().organizations,
		user,
	);

	const [selectedOrganizationId, setSelectedOrganizationId] =
		useState<string>();

	// List every member organization and load skills only for the selected
	// one: users can belong to hundreds, and a picker filtered by loaded
	// results shifts while requests settle.
	const selected =
		memberOrganizations.find(
			(organization) => organization.id === selectedOrganizationId,
		) ??
		memberOrganizations.find((organization) => organization.is_default) ??
		memberOrganizations.at(0);

	const organizationName = selected
		? selected.display_name || selected.name
		: "";

	const preferencesQuery = useQuery({
		...organizationSkillPreferences(selected?.id ?? ""),
		enabled: Boolean(selected),
	});

	const editablePreferences = preferencesQuery.data?.members_can_disable_skills
		? preferencesQuery.data
		: undefined;

	let organizationToolbar: React.ReactNode = null;
	if (preferencesQuery.isError && !preferencesQuery.data) {
		organizationToolbar = (
			<QueryErrorAlert
				query={preferencesQuery}
				retryLabel={`Retry loading your ${organizationName} skill choices`}
			/>
		);
	} else if (selected && editablePreferences) {
		organizationToolbar = (
			<OrganizationSkillsMasterSwitch
				organizationId={selected.id}
				preferences={editablePreferences}
			/>
		);
	}

	let organizationDescription: string;
	if (!preferencesQuery.data) {
		organizationDescription =
			"Skills available to you. Ask an organization admin to change the skill content.";
	} else if (editablePreferences) {
		organizationDescription =
			"Skills available to you. Turn off the ones you do not want in your chats. Ask an organization admin to change the skill content.";
	} else {
		organizationDescription =
			"Read-only skills available to you. Ask an organization admin to change them.";
	}

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
								organizations={memberOrganizations}
								organization={selected}
								onChange={(organization) =>
									setSelectedOrganizationId(organization.id)
								}
								showLabel={false}
							/>
						}
					/>
					<SkillsTable
						key={selected.id}
						scope={organizationSkillsScope(selected.id)}
						copy={{
							noun: "Organization skill",
							title: organizationName,
							description: organizationDescription,
							editorDescription:
								"Read-only organization skill. Ask an organization admin to change it.",
							archiveName: `${selected.name}-skills.zip`,
						}}
						access={readOnlySkillAccess}
						enabledOnly
						headerLevel="section"
						toolbar={organizationToolbar}
						isLoading={preferencesQuery.isPending}
						renderEnabledCell={
							editablePreferences
								? (skill) => (
										<OrganizationSkillPreferenceSwitch
											organizationId={selected.id}
											skill={skill}
											preferences={editablePreferences}
										/>
									)
								: undefined
						}
					/>
				</section>
			)}
		</div>
	);
};

export default AgentSettingsSkillsPage;
