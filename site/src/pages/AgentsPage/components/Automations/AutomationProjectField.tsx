import { useQuery } from "react-query";
import { chatProjects } from "#/api/queries/chatProjects";
import { SelectItem } from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import type { FormHelpers } from "#/utils/formUtils";

/** Project IDs are UUIDs, so this value cannot collide with one. */
export const NO_PROJECT = "none";

type AutomationProjectFieldProps = {
	organizationId: string;
	/** Its value is a project ID, or NO_PROJECT. */
	field: FormHelpers;
	onValueChange: (value: string) => void;
};

/** Selects the project that the chats of a new chat automation join. */
export const AutomationProjectField: React.FC<AutomationProjectFieldProps> = ({
	organizationId,
	field,
	onValueChange,
}) => {
	const value = String(field.value ?? NO_PROJECT);
	const projectsQuery = useQuery(chatProjects());
	const projects = (projectsQuery.data ?? []).filter(
		(project) => project.organization_id === organizationId,
	);
	// A stored project the viewer cannot list gets its own option, so the
	// trigger shows a label instead of rendering blank.
	const isUnlisted =
		value !== NO_PROJECT && !projects.some((project) => project.id === value);

	return (
		<SelectField
			field={field}
			label="Project"
			description={
				projectsQuery.isError
					? "Could not load projects."
					: projectsQuery.isLoading
						? "Loading projects…"
						: projects.length === 0
							? "You have no projects in this organization."
							: "Chats this automation creates join the selected project."
			}
			onValueChange={onValueChange}
		>
			<SelectItem value={NO_PROJECT}>No project</SelectItem>
			{projects.map((project) => (
				<SelectItem key={project.id} value={project.id}>
					{project.name}
				</SelectItem>
			))}
			{isUnlisted && (
				<SelectItem value={value}>
					{projectsQuery.isLoading ? "Loading project" : "Unknown project"}
				</SelectItem>
			)}
		</SelectField>
	);
};
