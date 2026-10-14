import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	FileCard,
	getAttachmentHref,
} from "./ChatConversation/AttachmentBlocks";
import { FileProbeProvider } from "./ChatConversation/FileProbeContext";
import type { ChatArtifact } from "./ChatConversation/messageHelpers";
import { SectionHeader } from "./SectionHeader";

type ChatArtifactsProps = {
	artifacts: readonly ChatArtifact[];
	evictedFileIds: ReadonlySet<string>;
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	hasFetchMoreError: boolean;
	onFetchMoreMessages: () => Promise<unknown>;
};

export const ChatArtifacts: React.FC<ChatArtifactsProps> = ({
	artifacts,
	evictedFileIds,
	hasMoreMessages,
	isFetchingMoreMessages,
	hasFetchMoreError,
	onFetchMoreMessages,
}) => {
	return (
		<section className="flex flex-col gap-3">
			<SectionHeader label="Artifacts" level="section" />
			{artifacts.length > 0 ? (
				<FileProbeProvider evictedFileIds={evictedFileIds}>
					<ul className="m-0 flex list-none flex-col gap-2 p-0">
						{artifacts.map(({ key, file }) => {
							const href = getAttachmentHref(file);
							return href ? (
								<li key={key}>
									<FileCard
										block={file}
										href={href}
										className="w-full max-w-none"
									/>
								</li>
							) : null;
						})}
					</ul>
				</FileProbeProvider>
			) : (
				<p className="m-0 text-sm text-content-secondary">
					{hasMoreMessages
						? "No artifacts in the loaded messages."
						: "No artifacts yet."}
				</p>
			)}
			{hasMoreMessages && (
				<Button
					variant="outline"
					size="sm"
					className="self-start"
					disabled={isFetchingMoreMessages}
					onClick={() => void onFetchMoreMessages()}
				>
					<Spinner loading={isFetchingMoreMessages} />
					{hasFetchMoreError
						? "Retry loading earlier messages"
						: "Load earlier messages"}
				</Button>
			)}
		</section>
	);
};
