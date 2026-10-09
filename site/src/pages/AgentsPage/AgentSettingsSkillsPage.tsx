import { useQueries } from "react-query";
import { organizationSkills } from "#/api/queries/skills";
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
	// A failed list stays visible so its table can show the error and a retry.
	const organizationsWithSkills = organizations.filter((_, index) => {
		const query = organizationSkillQueries[index];
		return Boolean(query.error) || query.data?.some((skill) => skill.enabled);
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
			{organizationsWithSkills.length > 0 && (
				<section className="flex flex-col gap-8">
					<SectionHeader
						label="From your organizations"
						description="Skills your organizations share with you. Your agents can use them in chats that belong to that organization."
					/>
					{organizationsWithSkills.map((organization) => (
						<SkillsTable
							key={organization.id}
							owner={{ type: "organization", organizationId: organization.id }}
							copy={{
								noun: "Organization skill",
								title: organization.display_name || organization.name,
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
					))}
				</section>
			)}
		</div>
	);
};

export default AgentSettingsSkillsPage;
