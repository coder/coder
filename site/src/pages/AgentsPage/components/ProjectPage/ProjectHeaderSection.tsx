import { PlusIcon } from "lucide-react";
import { useRef, useState } from "react";
import {
	type UseQueryResult,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { chatProjectACL, updateChatProject } from "#/api/queries/chatProjects";
import { user as userQuery } from "#/api/queries/users";
import type { ChatProject, ChatProjectACL } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AvatarGroup } from "./AvatarGroup";
import { ProjectActionsMenu } from "./ProjectActionsMenu";
import { ProjectPageHeader } from "./ProjectPageHeader";
import { ProjectSharingDialog } from "./ProjectSharingDialog";

type ProjectHeaderSectionProps = {
	readonly project: ChatProject;
};

/** The agent page header, wired to the owner, sharing, and name edits. */
export const ProjectHeaderSection: React.FC<ProjectHeaderSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const { user: me } = useAuthenticated();
	const { organizations, showOrganizations } = useDashboard();
	const isOwner = project.owner_id === me.id;
	const ownerQuery = useQuery({
		...userQuery(project.owner_id),
		enabled: !isOwner,
	});
	const owner = isOwner ? me : ownerQuery.data;
	let ownerLabel: string | undefined;
	if (owner) {
		ownerLabel = owner.name || owner.username;
	} else if (ownerQuery.isError) {
		ownerLabel = "Unknown";
	}
	const organization = organizations.find(
		(org) => org.id === project.organization_id,
	);
	let organizationLabel: string | undefined;
	if (showOrganizations) {
		organizationLabel = organization
			? organization.display_name || organization.name
			: "Unknown";
	}
	const updateMutation = useMutation(updateChatProject(queryClient));

	return (
		<ProjectPageHeader
			project={project}
			ownerLabel={ownerLabel}
			organizationLabel={organizationLabel}
			isOwnedByOtherUser={!isOwner}
			onSaveName={
				project.permissions.edit
					? (name) => updateMutation.mutateAsync({ project, request: { name } })
					: undefined
			}
			sharing={project.permissions.share && <ShareControl project={project} />}
			actions={<ProjectActionsMenu project={project} />}
		/>
	);
};

const ShareControl: React.FC<ProjectHeaderSectionProps> = ({ project }) => {
	const aclQuery = useQuery(chatProjectACL(project));
	const [isShareOpen, setIsShareOpen] = useState(false);
	const shareButtonRef = useRef<HTMLButtonElement>(null);
	return (
		<>
			<div className="flex items-center gap-3">
				<SharedWithAvatars aclQuery={aclQuery} />
				<Button
					ref={shareButtonRef}
					variant="outline"
					size="sm"
					onClick={() => setIsShareOpen(true)}
				>
					<PlusIcon />
					Share
				</Button>
			</div>
			{isShareOpen && (
				<ProjectSharingDialog
					project={project}
					onClose={() => {
						setIsShareOpen(false);
						requestAnimationFrame(() => shareButtonRef.current?.focus());
					}}
				/>
			)}
		</>
	);
};

type SharedWithAvatarsProps = {
	readonly aclQuery: UseQueryResult<ChatProjectACL>;
};

/**
 * Shows who the agent is shared with. A failed refetch keeps the avatars
 * already loaded; the failure notice only replaces them when there are none.
 */
const SharedWithAvatars: React.FC<SharedWithAvatarsProps> = ({ aclQuery }) => {
	const acl = aclQuery.data;
	if (!acl) {
		return aclQuery.isError ? (
			<span className="text-xs text-content-secondary">
				Sharing unavailable
			</span>
		) : (
			<Skeleton className="h-6 w-16" />
		);
	}
	const sharedWith = [
		...acl.groups.map((group) => ({
			id: group.id,
			name: group.display_name || group.name,
			src: group.avatar_url,
		})),
		...acl.users.map((user) => ({
			id: user.id,
			name: user.name || user.username,
			src: user.avatar_url,
		})),
	];
	if (sharedWith.length === 0) {
		return null;
	}
	const noun = sharedWith.length === 1 ? "user or group" : "users and groups";
	return (
		<AvatarGroup
			items={sharedWith}
			label={`Shared with ${sharedWith.length} ${noun}`}
		/>
	);
};
