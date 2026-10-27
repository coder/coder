import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { type ChatStore, useChatMessages } from "../ChatConversation/chatStore";
import { ArtifactView } from "./ArtifactView";
import { deriveSeenArtifactVersions } from "./artifactUtils";
import { useChatArtifact } from "./useChatArtifact";

type ArtifactPanelProps = {
	artifactId: string;
	/** The version to show. The latest version shows when it is unset. */
	version: number | undefined;
	onSelectVersion: (version: number) => void;
	chatId: string;
	store: ChatStore;
	onPrompt?: (text: string) => void;
	/** The viewer, for the Share popover. */
	currentUserId: string;
};

/** One artifact in its own right panel tab. The tab shows its title. */
export const ArtifactPanel: React.FC<ArtifactPanelProps> = ({
	artifactId,
	version,
	onSelectVersion,
	chatId,
	store,
	onPrompt,
	currentUserId,
}) => {
	const messages = useChatMessages(store);
	const seenVersion = deriveSeenArtifactVersions(messages).get(artifactId) ?? 0;
	const artifactQuery = useChatArtifact(artifactId, seenVersion);
	const { data } = artifactQuery;

	// A failed background refetch keeps the cached artifact on screen.
	if (data) {
		return (
			<ArtifactView
				artifact={data}
				selectedVersion={version}
				onSelectVersion={onSelectVersion}
				currentChatId={chatId}
				onPrompt={onPrompt}
				currentUserId={currentUserId}
				showTitle={false}
				showPageLink
			/>
		);
	}
	if (artifactQuery.isError) {
		return (
			<div className="p-4">
				<ErrorAlert error={artifactQuery.error} />
			</div>
		);
	}
	return <Loader className="h-full" label="Loading artifact" />;
};
