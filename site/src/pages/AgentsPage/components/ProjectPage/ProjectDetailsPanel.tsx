import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorStatus } from "#/api/errors";
import { chatProjectSkills } from "#/api/queries/chatProjectSkills";
import {
	chatProjectACL,
	chatProjectInstructions,
	deleteChatProjectInstructions,
	setChatProjectRole,
	updateChatProjectInstructions,
} from "#/api/queries/chatProjects";
import type { ChatProject, ChatProjectRole } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useDialogFocusReturn } from "../useDialogFocusReturn";
import { ProjectAccessDialog } from "./ProjectAccessDialog";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";
import { ProjectInstructionsDialog } from "./ProjectInstructionsDialog";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";
import {
	type AccessChangeCallbacks,
	buildAccessEntries,
	type ProjectAccessPrincipal,
} from "./projectAccess";

type ProjectDetailsPanelProps = {
	readonly project: ChatProject;
};

/** Loads a project's details and members, and edits its instructions, skills, and access. */
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
	const aclQuery = useQuery(chatProjectACL(project));
	const owner = aclQuery.data?.owner;
	const ownerLabel = isOwner
		? "You"
		: owner
			? owner.name || owner.username
			: aclQuery.isError
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
	// The ACL endpoints return 403 when chat sharing is disabled.
	const isSharingDisabled = getErrorStatus(aclQuery.error) === 403;
	const accessEntries = aclQuery.data
		? buildAccessEntries(aclQuery.data)
		: undefined;
	const [isAccessDialogOpen, setIsAccessDialogOpen] = useState(false);
	const accessFocusReturn = useDialogFocusReturn();
	const retryACL = () => void aclQuery.refetch();
	const roleMutation = useMutation(setChatProjectRole(queryClient));
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
	const isAccessChangeInFlight = useRef(false);

	const resetErrors = () => {
		updateMutation.reset();
		deleteMutation.reset();
	};

	const openEditor = () => {
		resetErrors();
		setIsEditorOpen(true);
	};

	const runMutation = (
		inFlight: React.RefObject<boolean>,
		reset: () => void,
		mutate: (onSettled: () => void) => void,
	) => {
		if (inFlight.current) {
			return;
		}
		inFlight.current = true;
		// Clear the previous action's error so only the latest one shows.
		reset();
		mutate(() => {
			inFlight.current = false;
		});
	};

	const resetAccessErrors = () => {
		// Resetting a pending mutation drops its per-call onSettled, which
		// would leave isAccessChangeInFlight set.
		if (!isAccessChangeInFlight.current) {
			roleMutation.reset();
		}
	};

	const setAccessRole = (
		principal: ProjectAccessPrincipal,
		role: ChatProjectRole,
		{ onSuccess, onSettled }: AccessChangeCallbacks,
	) =>
		runMutation(isAccessChangeInFlight, roleMutation.reset, (onDone) =>
			roleMutation.mutate(
				{ project, principal, role },
				{
					onSuccess,
					onSettled: () => {
						onDone();
						onSettled?.();
					},
				},
			),
		);

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
				members={
					isSharingDisabled
						? undefined
						: {
								entries: accessEntries,
								error: aclQuery.error,
								isRetrying: aclQuery.isFetching,
								onRetry: retryACL,
								canManage: project.permissions.share,
								onOpenAccess: (event) => {
									accessFocusReturn.rememberTrigger(event);
									resetAccessErrors();
									setIsAccessDialogOpen(true);
									// Show access changes made since the page loaded.
									void aclQuery.refetch();
								},
							}
				}
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
					runMutation(isMutationInFlight, resetErrors, (onSettled) =>
						updateMutation.mutate(
							{ instructions },
							{ onSuccess: () => setIsEditorOpen(false), onSettled },
						),
					)
				}
				onDelete={() =>
					runMutation(isMutationInFlight, resetErrors, (onSettled) =>
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
			<ProjectAccessDialog
				open={isAccessDialogOpen}
				onOpenChange={(open) => {
					resetAccessErrors();
					setIsAccessDialogOpen(open);
				}}
				entries={accessEntries}
				error={aclQuery.error}
				isRetrying={aclQuery.isFetching}
				onRetry={retryACL}
				onCloseAutoFocus={accessFocusReturn.restoreFocus}
				canManage={project.permissions.share}
				organizationId={project.organization_id}
				currentUserId={me.id}
				onAdd={setAccessRole}
				onChangeRole={setAccessRole}
				onRemove={(principal, callbacks) =>
					setAccessRole(principal, "", callbacks)
				}
				pendingPrincipalId={
					roleMutation.isPending
						? roleMutation.variables.principal.id
						: undefined
				}
				mutationError={roleMutation.error}
			/>
		</>
	);
};
