import { useQuery } from "react-query";
import { chatProjects } from "#/api/queries/chatProjects";
import { SelectItem } from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import type { FormHelpers } from "#/utils/formUtils";

/** Project IDs are UUIDs, so this value cannot collide with one. */
export const NO_PROJECT = "none";

type AutomationProjectFieldProps = {
	organizationId: string;
	field: FormHelpers;
	/** A project ID, or NO_PROJECT. */
	value: string;
	onValueChange: (value: string) => void;
};

/** Selects the project that the chats of a new chat automation join. */
export const AutomationProjectField: React.FC<AutomationProjectFieldProps> = ({
	organizationId,
	field,
	value,
	onValueChange,
}) => {
	const projectsQuery = useQuery(chatProjects());
	const projects = (projectsQuery.data ?? []).filter(
		(project) => project.organization_id === organizationId,
	);
	// A stored project the viewer cannot list stays selectable, so saving
	// the form does not silently remove it.
	const isUnlisted =
		value !== NO_PROJECT && !projects.some((project) => project.id === value);

	return (
		<SelectField
			field={field}
			label="Project"
			description={
				projectsQuery.isError
					? "Could not load projects."
					: "Chats this automation creates join the project."
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
					{projectsQuery.isLoading ? "Loading…" : "Unknown project"}
				</SelectItem>
			)}
		</SelectField>
	);
};
