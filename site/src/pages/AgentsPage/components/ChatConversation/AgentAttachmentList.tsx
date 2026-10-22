import { isAxiosError } from "axios";
import { useRef, useState } from "react";
import { useQuery } from "react-query";
import { chatFileText } from "#/api/queries/chats";
import { Spinner } from "#/components/Spinner/Spinner";
import { isRasterImageMediaType } from "../../utils/chatAttachments";
import {
	decodeInlineTextAttachment,
	getTextAttachmentErrorMessage,
} from "../../utils/fetchTextAttachment";
import { ImageThumbnail } from "../AttachmentPreview";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import {
	FileCard,
	getAttachmentDisplayName,
	isTextPreviewAttachmentMediaType,
} from "./AttachmentBlocks";
import type { AgentAttachment } from "./messageHelpers";
import { useAttachmentFailure } from "./useAttachmentFailure";

type AttachmentCardProps = {
	attachment: AgentAttachment;
	expired: boolean;
};

/** A thumbnail that failed to load leaves the image to download only. */
const ImageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment: { file, href },
	expired,
}) => {
	const [failure, onError] = useAttachmentFailure(file.file_id, href);
	const [isViewing, setIsViewing] = useState(false);
	const viewButtonRef = useRef<HTMLButtonElement>(null);

	if (failure.kind !== "idle") {
		return (
			<FileCard
				block={file}
				href={href}
				expired={expired || failure.kind === "expired"}
				fullWidth
			/>
		);
	}

	return (
		<>
			<FileCard
				block={file}
				href={href}
				expired={expired}
				fullWidth
				tile={
					<ImageThumbnail
						previewUrl={href}
						name=""
						className="size-full rounded-none border-0"
						loading="lazy"
						onError={onError}
					/>
				}
				onView={() => setIsViewing(true)}
				viewButtonRef={viewButtonRef}
			/>
			{isViewing && (
				<ImageLightbox
					src={href}
					onClose={() => setIsViewing(false)}
					onCloseAutoFocus={() => viewButtonRef.current?.focus()}
				/>
			)}
		</>
	);
};

type StoredTextAttachmentCardProps = AttachmentCardProps & { fileId: string };

/** Loads the file's text on the first view; a 404 shows it expired. */
const StoredTextAttachmentCard: React.FC<StoredTextAttachmentCardProps> = ({
	attachment: { file, href },
	expired,
	fileId,
}) => {
	const [isViewing, setIsViewing] = useState(false);
	const viewButtonRef = useRef<HTMLButtonElement>(null);
	const textQuery = useQuery({ ...chatFileText(fileId), enabled: isViewing });

	if (
		isAxiosError(textQuery.error) &&
		textQuery.error.response?.status === 404
	) {
		return <FileCard block={file} href={href} expired fullWidth />;
	}

	return (
		<>
			<FileCard
				block={file}
				href={href}
				expired={expired}
				fullWidth
				tile={
					textQuery.isLoading ? (
						<Spinner size="sm" loading className="text-content-secondary" />
					) : undefined
				}
				viewStatus={
					textQuery.isError
						? (getTextAttachmentErrorMessage(textQuery.error) ?? undefined)
						: undefined
				}
				onView={() => {
					setIsViewing(true);
					if (textQuery.isError) {
						void textQuery.refetch();
					}
				}}
				viewButtonRef={viewButtonRef}
			/>
			{isViewing && textQuery.data !== undefined && (
				<TextPreviewDialog
					content={textQuery.data}
					fileName={getAttachmentDisplayName(file)}
					mediaType={file.media_type}
					onClose={() => setIsViewing(false)}
					onCloseAutoFocus={() => viewButtonRef.current?.focus()}
				/>
			)}
		</>
	);
};

type InlineTextAttachmentCardProps = AttachmentCardProps & { data: string };

const InlineTextAttachmentCard: React.FC<InlineTextAttachmentCardProps> = ({
	attachment: { file, href },
	expired,
	data,
}) => {
	const [isViewing, setIsViewing] = useState(false);
	const viewButtonRef = useRef<HTMLButtonElement>(null);

	return (
		<>
			<FileCard
				block={file}
				href={href}
				expired={expired}
				fullWidth
				onView={() => setIsViewing(true)}
				viewButtonRef={viewButtonRef}
			/>
			{isViewing && (
				<TextPreviewDialog
					content={decodeInlineTextAttachment(data)}
					fileName={getAttachmentDisplayName(file)}
					mediaType={file.media_type}
					onClose={() => setIsViewing(false)}
					onCloseAutoFocus={() => viewButtonRef.current?.focus()}
				/>
			)}
		</>
	);
};

/** Images and text open in a viewer; other files only download. */
const AgentAttachmentCard: React.FC<AttachmentCardProps> = (props) => {
	const { file, href } = props.attachment;
	if (isRasterImageMediaType(file.media_type)) {
		return <ImageAttachmentCard {...props} />;
	}
	if (isTextPreviewAttachmentMediaType(file.media_type)) {
		if (file.file_id) {
			return <StoredTextAttachmentCard {...props} fileId={file.file_id} />;
		}
		if (file.data) {
			return <InlineTextAttachmentCard {...props} data={file.data} />;
		}
	}
	return (
		<FileCard block={file} href={href} expired={props.expired} fullWidth />
	);
};

type AgentAttachmentListProps = {
	attachments: readonly AgentAttachment[];
	isExpired: (fileId: string) => boolean;
};

/** Cards for agent attachments, in the given order. */
export const AgentAttachmentList: React.FC<AgentAttachmentListProps> = ({
	attachments,
	isExpired,
}) => (
	<ul className="flex flex-col gap-2">
		{attachments.map((attachment) => (
			<li key={attachment.key}>
				<AgentAttachmentCard
					attachment={attachment}
					// Inline attachments carry no file ID, so only stored files can expire.
					expired={
						attachment.file.file_id !== undefined &&
						isExpired(attachment.file.file_id)
					}
				/>
			</li>
		))}
	</ul>
);
