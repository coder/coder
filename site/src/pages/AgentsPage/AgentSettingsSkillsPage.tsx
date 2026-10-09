import { SkillsTable } from "./components/SkillsTable";

const AgentSettingsSkillsPage: React.FC = () => (
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
);

export default AgentSettingsSkillsPage;
