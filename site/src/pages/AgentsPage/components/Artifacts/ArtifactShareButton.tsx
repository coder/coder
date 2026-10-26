import { CheckIcon, LinkIcon, Share2Icon } from "lucide-react";
import type { ChatArtifact } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { useClipboard } from "#/hooks/useClipboard";
import { getArtifactLink } from "./artifactUtils";

type ArtifactShareButtonProps = {
	artifact: ChatArtifact;
	/** The version on screen. A link to it is offered when it is not the latest. */
	version: number | undefined;
	/** Names the owner "you" when it is the viewer. */
	currentUserId: string | undefined;
};

/**
 * Who can open the artifact link. Access follows the publishing chat: its
 * owner, and the members of its project.
 */
const describeAccess = (
	artifact: ChatArtifact,
	currentUserId: string | undefined,
): string => {
	if (artifact.project_name) {
		return `Members of ${artifact.project_name} can open this link.`;
	}
	return artifact.owner_id === currentUserId
		? "Only you can open this link. To share an artifact, publish it from a chat in a project."
		: `Only ${artifact.owner_username} can open this link.`;
};

/** Share: who can open the link, and Copy link for the latest or the shown version. */
export const ArtifactShareButton: React.FC<ArtifactShareButtonProps> = ({
	artifact,
	version,
	currentUserId,
}) => {
	const showVersionLink =
		version !== undefined && version !== artifact.latest_version;
	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button variant="subtle" size="icon" aria-label="Share artifact">
					<Share2Icon />
				</Button>
			</PopoverTrigger>
			<PopoverContent align="end" className="flex w-80 flex-col gap-3 p-4">
				<h3 className="m-0 text-sm font-semibold">Share artifact</h3>
				<p className="m-0 text-xs text-content-secondary">
					{describeAccess(artifact, currentUserId)}
				</p>
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
