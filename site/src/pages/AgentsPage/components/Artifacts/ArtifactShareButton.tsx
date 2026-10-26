import { CheckIcon, LinkIcon, Share2Icon, XIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	type ChatArtifactShareeKind,
	chatArtifactACL,
	setChatArtifactRole,
} from "#/api/queries/chatArtifacts";
import type { ChatArtifact } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import { useClipboard } from "#/hooks/useClipboard";
import { isGroup } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";
import { MemberIdentity } from "../ChatSharingPopover";
import { getArtifactLink, getArtifactShareCheck } from "./artifactUtils";

type ArtifactShareButtonProps = {
	artifact: ChatArtifact;
	/** The version on screen. A link to it is offered when it is not the latest. */
	version: number | undefined;
	/** Names the owner "you" when it is the viewer. */
	currentUserId: string;
};

type AccessOptions = {
	/** The viewer may share, so the viewer sees the shares. */
	canShare: boolean;
	/** The artifact is shared with at least one user or group. */
	hasShares: boolean;
};

/**
 * Who can open the artifact link: the owner, the members of its project,
 * and the users and groups it is shared with. A viewer who may not share
 * does not see the shares.
 */
const describeAccess = (
	artifact: ChatArtifact,
	currentUserId: string,
	{ canShare, hasShares }: AccessOptions,
): string => {
	const isOwner = artifact.owner_id === currentUserId;
	const project = artifact.project_name;
	if (!canShare && !isOwner) {
		return project
			? `Members of ${project} can open this link, and anyone ${artifact.owner_username} shares it with.`
			: `${artifact.owner_username} decides who can open this link.`;
	}
	const owner = isOwner ? "You" : artifact.owner_username;
	const shares = "the people and groups below";
	if (project && hasShares) {
		return `${owner}, members of ${project}, and ${shares} can open this link.`;
	}
	if (project) {
		return `${owner} and members of ${project} can open this link.`;
	}
	if (hasShares) {
		return `${owner} and ${shares} can open this link.`;
	}
	return `Only ${isOwner ? "you" : owner} can open this link.`;
};

/**
 * Share: who can open the link, sharing with users and groups for viewers
 * who may share, and Copy link for the latest or the shown version.
 */
export const ArtifactShareButton: React.FC<ArtifactShareButtonProps> = ({
	artifact,
	version,
	currentUserId,
}) => {
	const [open, setOpen] = useState(false);
	const shareCheck = useQuery({
		...checkAuthorization(getArtifactShareCheck(artifact)),
		enabled: open,
	});
	const showVersionLink =
		version !== undefined && version !== artifact.latest_version;

	let access: React.ReactNode;
	if (shareCheck.error) {
		access = <ErrorAlert error={shareCheck.error} />;
	} else if (!shareCheck.data) {
		access = (
			<Spinner loading label="Loading sharing" className="self-center" />
		);
	} else if (shareCheck.data.canShare) {
		access = (
			<ArtifactSharing artifact={artifact} currentUserId={currentUserId} />
		);
	} else {
		access = (
			<p className="m-0 text-xs text-content-secondary">
				{describeAccess(artifact, currentUserId, {
					canShare: false,
					hasShares: false,
				})}
			</p>
		);
	}

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button variant="subtle" size="icon" aria-label="Share artifact">
					<Share2Icon />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="flex w-96 max-w-[calc(100vw-2rem)] flex-col gap-3 p-4"
			>
				<h3 className="m-0 text-sm font-semibold">Share artifact</h3>
				{access}
				<div className="flex flex-wrap gap-2">
					<CopyLinkButton
						link={getArtifactLink(artifact.id)}
						label="Copy link"
					/>
					{showVersionLink && (
						<CopyLinkButton
							link={getArtifactLink(artifact.id, version)}
							label={`Copy link to v${version}`}
						/>
					)}
				</div>
			</PopoverContent>
		</Popover>
	);
};

type ArtifactSharingProps = {
	artifact: ChatArtifact;
	currentUserId: string;
};

/** The users and groups an artifact is shared with, and a form to add one. */
const ArtifactSharing: React.FC<ArtifactSharingProps> = ({
	artifact,
	currentUserId,
}) => {
	const queryClient = useQueryClient();
	const aclQuery = useQuery(chatArtifactACL(artifact.id));
	const roleMutation = useMutation(setChatArtifactRole(queryClient));
	const [selected, setSelected] = useState<UserOrGroupAutocompleteValue>(null);

	if (aclQuery.error) {
		return <ErrorAlert error={aclQuery.error} />;
	}
	if (!aclQuery.data) {
		return <Spinner loading label="Loading sharing" className="self-center" />;
	}
	const { users, groups } = aclQuery.data;
	const hasShares = users.length + groups.length > 0;

	const share = (event: React.FormEvent) => {
		event.preventDefault();
		if (!selected || roleMutation.isPending) {
			return;
		}
		roleMutation.mutate(
			{
				artifactId: artifact.id,
				id: selected.id,
				kind: isGroup(selected) ? "group" : "user",
				role: "read",
			},
			{ onSuccess: () => setSelected(null) },
		);
	};
	const stopSharing = (id: string, kind: ChatArtifactShareeKind) => {
		roleMutation.mutate({ artifactId: artifact.id, id, kind, role: "" });
	};

	return (
		<>
			<p className="m-0 text-xs text-content-secondary">
				{describeAccess(artifact, currentUserId, { canShare: true, hasShares })}
				{hasShares &&
					" People you share with can view every version, but not the chats that published them."}
			</p>
			<form onSubmit={share} className="flex items-center gap-2">
				<UserOrGroupAutocomplete
					value={selected}
					onChange={setSelected}
					organizationId={artifact.organization_id}
					exclude={[...users, ...groups, { id: artifact.owner_id }]}
					className="min-w-0 flex-1"
				/>
				<Button
					type="submit"
					size="sm"
					disabled={!selected || roleMutation.isPending}
				>
					<Spinner loading={roleMutation.isPending} />
					Share
				</Button>
			</form>
			{roleMutation.error ? <ErrorAlert error={roleMutation.error} /> : null}
			{hasShares && (
				<ul
					aria-label="Shared with"
					className="m-0 flex max-h-60 list-none flex-col gap-2 overflow-y-auto p-0"
				>
					{groups.map((group) => (
						<SharedWithRow
							key={group.id}
							name={group.display_name || group.name}
							disabled={roleMutation.isPending}
							onRemove={() => stopSharing(group.id, "group")}
						>
							<MemberIdentity kind="group" group={group} />
						</SharedWithRow>
					))}
					{users.map((user) => (
						<SharedWithRow
							key={user.id}
							name={user.username}
							disabled={roleMutation.isPending}
							onRemove={() => stopSharing(user.id, "user")}
						>
							<MemberIdentity kind="user" user={user} />
						</SharedWithRow>
					))}
				</ul>
			)}
		</>
	);
};

type SharedWithRowProps = {
	name: string;
	disabled: boolean;
	onRemove: () => void;
	children: React.ReactNode;
};

const SharedWithRow: React.FC<SharedWithRowProps> = ({
	name,
	disabled,
	onRemove,
	children,
}) => (
	<li className="flex items-center justify-between gap-2">
		<div className="min-w-0">{children}</div>
		<Button
			variant="subtle"
			size="icon"
			aria-label={`Stop sharing with ${name}`}
			disabled={disabled}
			onClick={onRemove}
		>
			<XIcon />
		</Button>
	</li>
);

type CopyLinkButtonProps = {
	link: string;
	label: string;
};

const CopyLinkButton: React.FC<CopyLinkButtonProps> = ({ link, label }) => {
	const { copyToClipboard, showCopiedSuccess } = useClipboard();
	return (
		<Button
			size="sm"
			variant="outline"
			onClick={() => {
				void copyToClipboard(link);
			}}
		>
			{showCopiedSuccess ? <CheckIcon /> : <LinkIcon />}
			{showCopiedSuccess ? "Copied" : label}
		</Button>
	);
};
