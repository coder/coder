import { ShapesIcon } from "lucide-react";
import {
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { type ChatStore, useChatMessages } from "../ChatConversation/chatStore";
import { useArtifactTabs } from "./ArtifactTabsContext";
import {
	artifactKindIcons,
	deriveArtifactIds,
	deriveSeenArtifactVersions,
	kindOf,
} from "./artifactUtils";
import { useChatArtifact } from "./useChatArtifact";

type ArtifactsSubMenuProps = {
	/** The chat's messages. Only the artifacts of loaded messages show. */
	store: ChatStore;
};

/**
 * An "Artifacts" submenu for the right panel's add menu. Each item opens an
 * artifact of the chat in its own tab. Chats without artifacts show nothing.
 */
export const ArtifactsSubMenu: React.FC<ArtifactsSubMenuProps> = ({
	store,
}) => {
	const messages = useChatMessages(store);
	const artifactIds = deriveArtifactIds(messages);
	if (artifactIds.length === 0) {
		return null;
	}
	const seenVersions = deriveSeenArtifactVersions(messages);
	return (
		<>
			<DropdownMenuSeparator className="my-1" />
			<DropdownMenuSub>
				<DropdownMenuSubTrigger>
					<ShapesIcon />
					Artifacts
				</DropdownMenuSubTrigger>
				<DropdownMenuSubContent className="w-56 p-1 [&_[role=menuitem]]:py-1 [&_[role=menuitem]]:text-xs [&_svg]:size-3.5!">
					{artifactIds.map((artifactId) => (
						<ArtifactMenuItem
							key={artifactId}
							artifactId={artifactId}
							seenVersion={seenVersions.get(artifactId) ?? 0}
						/>
					))}
				</DropdownMenuSubContent>
			</DropdownMenuSub>
		</>
	);
};

type ArtifactMenuItemProps = {
	artifactId: string;
	/** The highest version that a tool result in the chat names. */
	seenVersion: number;
};

const ArtifactMenuItem: React.FC<ArtifactMenuItemProps> = ({
	artifactId,
	seenVersion,
}) => {
	const { onOpenArtifact } = useArtifactTabs();
	const { data, isError } = useChatArtifact(artifactId, seenVersion);

	if (!data) {
		return (
			<DropdownMenuItem disabled>
				{isError ? "Unavailable artifact" : "Loading…"}
			</DropdownMenuItem>
		);
	}
	const title = data.title || "Untitled artifact";
	const Icon =
		artifactKindIcons[kindOf(data.latest_name, data.latest_media_type)];
	return (
		<DropdownMenuItem
			disabled={!onOpenArtifact}
			onSelect={() => onOpenArtifact?.(artifactId, title)}
		>
			<Icon />
			<span className="min-w-0 flex-1 truncate">{title}</span>
			<span className="text-content-secondary">v{data.latest_version}</span>
		</DropdownMenuItem>
	);
};
