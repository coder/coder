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
	ViewableFileCard,
} from "./AttachmentBlocks";
import type { AgentAttachment } from "./messageHelpers";
import { useAttachmentFailure } from "./useAttachmentFailure";

type AttachmentCardProps = {
	attachment: AgentAttachment;
	expired: boolean;
};

/** A card's viewer: whether it is open, and focus back on the card on close. */
const useAttachmentViewer = () => {
	const [isViewing, setIsViewing] = useState(false);
	const viewButtonRef = useRef<HTMLButtonElement>(null);
	return {
		isViewing,
		open: () => setIsViewing(true),
		viewButtonRef,
		dialogProps: {
			onClose: () => setIsViewing(false),
			onCloseAutoFocus: () => viewButtonRef.current?.focus(),
		},
	};
};

/**
 * A thumbnail that fails to load shows the download card, or the expired tile
 * once the probe finds the file deleted, since the viewer would fail too.
 */
const ImageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment: { file, href },
	expired,
}) => {
	const [failure, onError] = useAttachmentFailure(file.file_id, href);
	const viewer = useAttachmentViewer();

	if (expired || failure.kind !== "idle") {
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
			<ViewableFileCard
				block={file}
				href={href}
				tile={
					<ImageThumbnail
						previewUrl={href}
						name=""
						className="size-full rounded-none border-0"
						loading="lazy"
						onError={onError}
					/>
				}
				onView={viewer.open}
				viewButtonRef={viewer.viewButtonRef}
			/>
			{viewer.isViewing && <ImageLightbox src={href} {...viewer.dialogProps} />}
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
	const viewer = useAttachmentViewer();
	const textQuery = useQuery({
		...chatFileText(fileId),
		enabled: viewer.isViewing,
	});
	const isLoadingText = textQuery.isFetching && textQuery.data === undefined;
	const loadError = textQuery.isError
		? (getTextAttachmentErrorMessage(textQuery.error) ?? undefined)
		: undefined;

	if (
		expired ||
		(isAxiosError(textQuery.error) && textQuery.error.response?.status === 404)
	) {
		return <FileCard block={file} href={href} expired fullWidth />;
	}

	return (
		<>
			<ViewableFileCard
				block={file}
				href={href}
				tile={
					isLoadingText ? (
						<Spinner size="sm" loading className="text-content-secondary" />
					) : undefined
				}
				status={isLoadingText ? "Loading file" : loadError}
				onView={() => {
					viewer.open();
					// A failed load leaves the query enabled, so only a refetch
					// retries it.
					if (textQuery.isError) {
						void textQuery.refetch();
					}
				}}
				viewButtonRef={viewer.viewButtonRef}
			/>
			{viewer.isViewing && textQuery.data !== undefined && (
				<TextPreviewDialog
					content={textQuery.data}
					fileName={getAttachmentDisplayName(file)}
					mediaType={file.media_type}
					{...viewer.dialogProps}
				/>
			)}
		</>
	);
};

type InlineTextAttachmentCardProps = {
	attachment: AgentAttachment;
	data: string;
};

const InlineTextAttachmentCard: React.FC<InlineTextAttachmentCardProps> = ({
	attachment: { file, href },
	data,
}) => {
	const viewer = useAttachmentViewer();

	return (
		<>
			<ViewableFileCard
				block={file}
				href={href}
				onView={viewer.open}
				viewButtonRef={viewer.viewButtonRef}
			/>
			{viewer.isViewing && (
				<TextPreviewDialog
					content={decodeInlineTextAttachment(data)}
					fileName={getAttachmentDisplayName(file)}
					mediaType={file.media_type}
					{...viewer.dialogProps}
				/>
			)}
		</>
	);
};

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
			return (
				<InlineTextAttachmentCard
					attachment={props.attachment}
					data={file.data}
				/>
			);
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
