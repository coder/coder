import type { Chat } from "#/api/typesGenerated";
import { Separator } from "#/components/Separator/Separator";
import { ChatTreeNode } from "../tree/ChatTreeNode";
import { SortableChatTreeNode } from "../tree/SortableChatTreeNode";
import {
	ChatSectionHeader,
	getSectionToggleTestId,
	PINNED_SECTION_KEY,
} from "./ChatSectionHeader";
import { ProjectFolders, type ProjectFoldersProps } from "./ProjectFolders";

type PinnedSectionProps = {
	/** The user's pinned chats, in pin order. */
	readonly pinnedChats: readonly Chat[];
	readonly canReorderChats: boolean;
	/** The pinned project folders, or undefined while projects are off. */
	readonly projectFolders: ProjectFoldersProps | undefined;
	readonly expanded: boolean;
	readonly onToggle: () => void;
};

/** Lists the pinned chats, then the pinned project folders. */
export const PinnedSection: React.FC<PinnedSectionProps> = ({
	pinnedChats,
	canReorderChats,
	projectFolders,
	expanded,
	onToggle,
}) => {
	const pinnedProjectCount = projectFolders?.pinnedProjects.length ?? 0;
	return (
		<div className="not-first:mt-3">
			<ChatSectionHeader
				label={PINNED_SECTION_KEY}
				count={pinnedChats.length + pinnedProjectCount}
				expanded={expanded}
				onToggle={onToggle}
				testId={getSectionToggleTestId(PINNED_SECTION_KEY)}
			/>
			{expanded && (
				<>
					<div className="flex flex-col gap-0.5">
						{pinnedChats.map((chat, index) =>
							canReorderChats ? (
								<SortableChatTreeNode key={chat.id} chat={chat} index={index} />
							) : (
								<ChatTreeNode key={chat.id} chat={chat} />
							),
						)}
					</div>
					{pinnedChats.length > 0 && pinnedProjectCount > 0 && (
						<div className="flex items-center gap-2 py-1.5 text-xs text-content-secondary">
							<Separator
								decorative={false}
								aria-label="Projects"
								className="flex-1"
							/>
							<span aria-hidden="true">Projects</span>
							<Separator className="flex-1" />
						</div>
					)}
					{projectFolders && <ProjectFolders {...projectFolders} />}
				</>
			)}
		</div>
	);
};
