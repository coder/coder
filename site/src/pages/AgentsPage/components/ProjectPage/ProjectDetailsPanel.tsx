import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectInstructions,
	deleteChatProjectInstructions,
	updateChatProjectInstructions,
} from "#/api/queries/chatProjects";
import { user } from "#/api/queries/users";
import type { ChatProject } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";
import { ProjectInstructionsDialog } from "./ProjectInstructionsDialog";

type ProjectDetailsPanelProps = {
	readonly project: ChatProject;
};

/** Loads a project's details and edits its instructions. */
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
				instructions={instructionsQuery.data}
				error={instructionsQuery.error}
				onRetry={() => void instructionsQuery.refetch()}
				onEditInstructions={openEditor}
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
		</>
	);
};
