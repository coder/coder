import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { chatProjectSkills } from "#/api/queries/chatProjectSkills";
import {
	chatProjectInstructions,
	deleteChatProjectInstructions,
	updateChatProjectInstructions,
} from "#/api/queries/chatProjects";
import { user } from "#/api/queries/users";
import type { ChatProject } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useDialogFocusReturn } from "../useDialogFocusReturn";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";
import { ProjectInstructionsDialog } from "./ProjectInstructionsDialog";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";

type ProjectDetailsPanelProps = {
	readonly project: ChatProject;
};

/** Loads a project's details and edits its instructions and skills. */
export const ProjectDetailsPanel: React.FC<ProjectDetailsPanelProps> = ({
	project,
}) => (
	// Route changes between projects reuse this component, so remount it
	// to drop an open editor and its draft instead of saving that draft
	// to the next project.
	<ProjectDetailsPanelContent key={project.id} project={project} />
);

const ProjectDetailsPanelContent: React.FC<ProjectDetailsPanelProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const { user: me } = useAuthenticated();
	const { organizations, showOrganizations } = useDashboard();
	const isOwner = project.owner_id === me.id;
	const ownerQuery = useQuery({ ...user(project.owner_id), enabled: !isOwner });
	const ownerLabel = isOwner
		? "You"
		: ownerQuery.data
			? ownerQuery.data.name || ownerQuery.data.username
			: ownerQuery.isError
				? "Unknown"
				: undefined;
	const organization = organizations.find(
		(org) => org.id === project.organization_id,
	);
	const organizationLabel = showOrganizations
		? organization
			? organization.display_name || organization.name
			: "Unknown"
		: undefined;
	const instructionsQuery = useQuery(chatProjectInstructions(project));
	const skillsQuery = useQuery(
		chatProjectSkills(project.organization_id, project.id),
	);
	const [isSkillsDialogOpen, setIsSkillsDialogOpen] = useState(false);
	const skillsFocusReturn = useDialogFocusReturn();
	const updateMutation = useMutation(
		updateChatProjectInstructions(queryClient, project),
	);
	const deleteMutation = useMutation(
		deleteChatProjectInstructions(queryClient, project),
	);
	const [isEditorOpen, setIsEditorOpen] = useState(false);
	// isPending reaches the dialog only after a render, so a fast double
	// click could otherwise submit the same change twice.
	const isMutationInFlight = useRef(false);

	const resetErrors = () => {
		updateMutation.reset();
		deleteMutation.reset();
	};

	const openEditor = () => {
		resetErrors();
		setIsEditorOpen(true);
	};

	const runMutation = (mutate: (onSettled: () => void) => void) => {
		if (isMutationInFlight.current) {
			return;
		}
		isMutationInFlight.current = true;
		// Clear the previous action's error so only the latest one shows.
		resetErrors();
		mutate(() => {
			isMutationInFlight.current = false;
		});
	};

	return (
		<>
			<ProjectDetailsPanelView
				ownerLabel={ownerLabel}
				createdAt={project.created_at}
				organizationLabel={organizationLabel}
				skillCount={skillsQuery.data?.length}
				disabledSkillCount={
					skillsQuery.data?.filter((skill) => !skill.enabled).length
				}
				skillsError={skillsQuery.error}
				onOpenSkills={(event) => {
					skillsFocusReturn.rememberTrigger(event);
					setIsSkillsDialogOpen(true);
				}}
				instructions={instructionsQuery.data}
				error={instructionsQuery.error}
				onRetry={() => void instructionsQuery.refetch()}
				onEditInstructions={project.permissions.update ? openEditor : undefined}
			/>
			<ProjectInstructionsDialog
				open={isEditorOpen}
				onOpenChange={setIsEditorOpen}
				instructions={instructionsQuery.data?.instructions ?? ""}
				isSaving={updateMutation.isPending}
				isDeleting={deleteMutation.isPending}
				saveError={updateMutation.error}
				deleteError={deleteMutation.error}
				onResetErrors={() => {
					// The error fields lag a render, so also check the ref: resetting
					// a pending mutation drops its per-call onSuccess and onSettled.
					if (
						!isMutationInFlight.current &&
						(updateMutation.error || deleteMutation.error)
					) {
						resetErrors();
					}
				}}
				onSave={(instructions) =>
					runMutation((onSettled) =>
						updateMutation.mutate(
							{ instructions },
							{ onSuccess: () => setIsEditorOpen(false), onSettled },
						),
					)
				}
				onDelete={() =>
					runMutation((onSettled) =>
						deleteMutation.mutate(undefined, {
							onSuccess: () => setIsEditorOpen(false),
							onSettled,
						}),
					)
				}
			/>
			<ProjectSkillsDialog
				project={project}
				open={isSkillsDialogOpen}
				onOpenChange={setIsSkillsDialogOpen}
				canEdit={project.permissions.update}
				onCloseAutoFocus={skillsFocusReturn.restoreFocus}
			/>
		</>
	);
};
