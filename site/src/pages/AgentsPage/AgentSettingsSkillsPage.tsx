import { useQueries } from "react-query";
import { organizationSkills } from "#/api/queries/skills";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { SectionHeader } from "./components/SectionHeader";
import { SkillsTable } from "./components/SkillsTable";

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

	return (
		<div className="flex flex-col gap-12">
			<SkillsTable
				owner={{ type: "user", user: "me" }}
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
				canEdit
			/>
			{visibleOrganizations.length > 0 && (
				<section className="flex flex-col gap-8">
					<SectionHeader
						label="From your organizations"
						description="Organization skills your agents can use in chats that belong to that organization. Members get the skills shared with them. Site owners, organization admins, and auditors get every enabled skill."
					/>
					{visibleOrganizations.map(
						({ organization, query, hasEnabledSkills }) => {
							const organizationName =
								organization.display_name || organization.name;
							// The table's own observer would refetch a failed list on
							// mount and clear its error, so a failed list renders here.
							if (query.isError && !hasEnabledSkills) {
								return (
									<div key={organization.id} className="flex flex-col gap-4">
										<SectionHeader level="section" label={organizationName} />
										<ErrorAlert
											error={query.error}
											actions={
												<Button
													size="sm"
													variant="outline"
													aria-label={`Retry loading ${organizationName} skills`}
													onClick={() => {
														void query.refetch();
													}}
												>
													Retry
												</Button>
											}
										/>
									</div>
								);
							}
							return (
								<SkillsTable
									key={organization.id}
									owner={{
										type: "organization",
										organizationId: organization.id,
									}}
									copy={{
										noun: "Organization skill",
										title: organizationName,
										description:
											"Read-only skills available to you. Ask an organization admin to change them.",
										emptyDescription: "",
										editorDescription: "",
										archiveName: `${organization.name}-skills.zip`,
									}}
									canEdit={false}
									enabledOnly
									headerLevel="section"
								/>
							);
						},
					)}
				</section>
			)}
		</div>
	);
};

export default AgentSettingsSkillsPage;
