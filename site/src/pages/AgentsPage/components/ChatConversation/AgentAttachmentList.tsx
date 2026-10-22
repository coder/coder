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
import { useDialogFocusReturn } from "../useDialogFocusReturn";
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
	const { rememberTrigger, restoreFocus } = useDialogFocusReturn();
	return {
		isViewing,
		open: (event: React.MouseEvent<HTMLButtonElement>) => {
			rememberTrigger(event);
			setIsViewing(true);
		},
		dialogProps: {
			onClose: () => setIsViewing(false),
			onCloseAutoFocus: restoreFocus,
		},
	};
};

/**
 * A ref for each root a card swaps between, so that when a failure swaps out
 * the root holding focus, its replacement takes focus, not the page body.
 */
const useSwapFocusRef = () => {
	const hadFocusRef = useRef(false);
	return (node: HTMLElement | null) => {
		if (!node) {
			return;
		}
		if (hadFocusRef.current && document.activeElement === document.body) {
			node.focus();
		}
		hadFocusRef.current = false;
		// React runs ref cleanups before it removes the node, so focus inside
		// it has not fallen to the page body yet.
		return () => {
			hadFocusRef.current = node.contains(document.activeElement);
		};
	};
};

const ImageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment: { file, href },
	expired,
}) => {
	const [failure, onError] = useAttachmentFailure(file.file_id, href);
	const viewer = useAttachmentViewer();
	const swapFocusRef = useSwapFocusRef();

	// The viewer would show the same broken image as a failed thumbnail.
	if (expired || failure.kind !== "idle") {
		return (
			<FileCard
				block={file}
				href={href}
				expired={expired || failure.kind === "expired"}
				fullWidth
				ref={swapFocusRef}
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
				ref={swapFocusRef}
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
	const swapFocusRef = useSwapFocusRef();
	const textQuery = useQuery({
		...chatFileText(fileId),
		enabled: viewer.isViewing,
	});
	const isExpired =
		expired ||
		(isAxiosError(textQuery.error) && textQuery.error.response?.status === 404);
	const isLoadingText = textQuery.isFetching && textQuery.data === undefined;
	const loadError = textQuery.isError
		? (getTextAttachmentErrorMessage(textQuery.error) ?? undefined)
		: undefined;

	if (isExpired) {
		return (
			<FileCard block={file} href={href} expired fullWidth ref={swapFocusRef} />
		);
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
				onView={(event) => {
					viewer.open(event);
					// A failed load leaves the query enabled, so only a refetch
					// retries it.
					if (textQuery.isError) {
						void textQuery.refetch();
					}
				}}
				ref={swapFocusRef}
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
			<ViewableFileCard block={file} href={href} onView={viewer.open} />
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
