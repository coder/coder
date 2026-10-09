import { useQueries } from "react-query";
import { organizationSkills } from "#/api/queries/skills";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { SectionHeader } from "./components/SectionHeader";
import { SkillsTable } from "./components/SkillsTable";

const AgentSettingsSkillsPage: React.FC = () => {
	const { organizations } = useDashboard();
	const organizationSkillQueries = useQueries({
		queries: organizations.map((organization) =>
			organizationSkills(organization.id),
		),
	});
	const visibleOrganizations = organizations.flatMap((organization, index) => {
		const query = organizationSkillQueries[index];
		const hasEnabledSkills = Boolean(
			query.data?.some((skill) => skill.enabled),
		);
		return hasEnabledSkills || query.isError
			? [{ organization, query, hasEnabledSkills }]
			: [];
	});

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
						description="Skills your organizations share with you. Your agents can use them in chats that belong to that organization."
					/>
					{visibleOrganizations.map(
						({ organization, query, hasEnabledSkills }) => {
							const organizationName =
								organization.display_name || organization.name;
							// The table's own observer would refetch a failed list on
							// mount and clear its error, so a failed list renders here.
							if (!hasEnabledSkills) {
								return (
									<div key={organization.id} className="flex flex-col gap-4">
										<SectionHeader level="section" label={organizationName} />
										<ErrorAlert
											error={query.error}
											actions={
												<Button
													size="sm"
													variant="outline"
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
											"Read-only skills shared with you. Ask an organization admin to change them.",
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
