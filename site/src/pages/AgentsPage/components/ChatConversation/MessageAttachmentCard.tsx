import { isAxiosError } from "axios";
import { cn } from "cn";
import { AlertTriangleIcon, DownloadIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
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
	FileCardDetails,
	fileAttachmentFailureLabels,
	getAttachmentDisplayName,
	getAttachmentDownload,
	isTextPreviewAttachmentMediaType,
} from "./AttachmentBlocks";
import { AttachmentDownloadLink } from "./AttachmentDownloadLink";
import type { MessageAttachment } from "./messageHelpers";

type AttachmentFile = MessageAttachment["file"];

/** A card's viewer, which puts focus back on the card when it closes. */
const useAttachmentViewer = () => {
	const [isViewing, setIsViewing] = useState(false);
	const { rememberTrigger, restoreFocus } = useDialogFocusReturn();
	const close = () => setIsViewing(false);
	return {
		isViewing,
		open: (event: React.MouseEvent<HTMLButtonElement>) => {
			rememberTrigger(event);
			setIsViewing(true);
		},
		close,
		dialogProps: { onClose: close, onCloseAutoFocus: restoreFocus },
	};
};

type AttachmentDialogProps = ReturnType<
	typeof useAttachmentViewer
>["dialogProps"];

type ViewableFileCardProps = {
	file: AttachmentFile;
	href: string;
	onView: (event: React.MouseEvent<HTMLButtonElement>) => void;
	/** Shown in place of the file type badge. */
	tile?: React.ReactNode;
	/** Shown under the name in place of "View file". */
	status?: string | null;
	/**
	 * The file is gone: the button keeps focus but no longer views or
	 * downloads it.
	 */
	expired?: boolean;
	ref?: React.RefCallback<HTMLElement>;
};

/**
 * Speech input matches visible text, so the button's accessible name keeps
 * the status as well as the file name.
 */
const ViewableFileCard: React.FC<ViewableFileCardProps> = ({
	file,
	href,
	onView,
	tile,
	status,
	expired = false,
	ref,
}) => {
	const nameId = useId();
	const statusId = useId();

	return (
		<div
			ref={ref}
			className={cn(
				"inline-flex h-16 w-full items-center gap-1 rounded-md border border-solid border-border bg-surface-tertiary pr-2 transition-colors has-[>button:focus-visible]:ring-2 has-[>button:focus-visible]:ring-content-link",
				!expired && "has-[>button:hover]:bg-surface-quaternary",
			)}
		>
			<button
				type="button"
				aria-labelledby={`${nameId} ${statusId}`}
				aria-disabled={expired || undefined}
				className="flex h-full min-w-0 flex-1 items-center gap-3 border-0 bg-transparent py-2 pl-3 text-left focus-visible:outline-hidden aria-disabled:cursor-default"
				onClick={expired ? undefined : onView}
			>
				<FileCardDetails
					block={file}
					tile={
						expired ? (
							<AlertTriangleIcon
								aria-hidden="true"
								className="size-icon-sm text-content-warning"
							/>
						) : (
							tile
						)
					}
					status={
						expired
							? fileAttachmentFailureLabels.expired
							: (status ?? "View file")
					}
					nameId={nameId}
					statusId={statusId}
				/>
			</button>
			{!expired && (
				<AttachmentDownloadLink
					{...getAttachmentDownload(file, href)}
					className="flex size-6 shrink-0 items-center justify-center rounded-md text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<DownloadIcon aria-hidden="true" className="size-4" />
				</AttachmentDownloadLink>
			)}
		</div>
	);
};

type TextAttachmentDialogProps = AttachmentDialogProps & {
	file: AttachmentFile;
	content: string | undefined;
};

const TextAttachmentDialog: React.FC<TextAttachmentDialogProps> = ({
	file,
	content,
	...dialogProps
}) => (
	<TextPreviewDialog
		content={content}
		fileName={getAttachmentDisplayName(file)}
		mediaType={file.media_type}
		{...dialogProps}
	/>
);

type AttachmentCardProps = {
	attachment: MessageAttachment;
};

/**
 * A thumbnail that fails to load swaps the card for a download card, since
 * the viewer would show the same broken image.
 */
const ImageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment: { file, href },
}) => {
	const [hasFailed, setHasFailed] = useState(false);
	const viewer = useAttachmentViewer();
	const hadFocusRef = useRef(false);
	// Both cards take this ref, so when the swap removes the focused card,
	// the download card takes focus instead of the page body.
	const swapFocusRef = (node: HTMLElement) => {
		if (hadFocusRef.current && document.activeElement === document.body) {
			node.focus();
		}
		// React runs ref cleanups before it removes the node, so focus inside
		// it has not fallen to the page body yet.
		return () => {
			// An open viewer holds focus in its portal instead.
			hadFocusRef.current =
				viewer.isViewing || node.contains(document.activeElement);
		};
	};

	if (hasFailed) {
		return (
			<FileCard
				block={file}
				href={href}
				expired={false}
				fullWidth
				ref={swapFocusRef}
			/>
		);
	}

	return (
		<>
			<ViewableFileCard
				file={file}
				href={href}
				tile={
					<ImageThumbnail
						previewUrl={href}
						alt=""
						className="size-full rounded-none border-0"
						loading="lazy"
						onError={() => setHasFailed(true)}
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

/**
 * The text endpoint's error bodies stay unparsed strings, which
 * getErrorStatus does not read.
 */
export const isExpiredFileError = (error: unknown) =>
	isAxiosError(error) && error.response?.status === 404;

/** A text-preview file's stored ID; undefined for inline text and other media types. */
export const getStoredTextFileId = (file: AttachmentFile) =>
	isTextPreviewAttachmentMediaType(file.media_type) ? file.file_id : undefined;

const StoredTextAttachmentCard: React.FC<StoredTextAttachmentCardProps> = ({
	attachment: { file, href },
	fileId,
}) => {
	const viewer = useAttachmentViewer();
	const textQuery = useQuery(chatFileText(fileId));
	const loadError = textQuery.isError
		? getTextAttachmentErrorMessage(textQuery.error)
		: null;

	return (
		<>
			<ViewableFileCard
				file={file}
				href={href}
				expired={isExpiredFileError(textQuery.error)}
				tile={
					textQuery.isLoading ? (
						<Spinner size="sm" loading className="text-content-secondary" />
					) : undefined
				}
				status={textQuery.isLoading ? "Loading file" : loadError}
				onView={(event) => {
					// The preview opens before the text loads, so no other file
					// can open a second one meanwhile.
					viewer.open(event);
					if (textQuery.data !== undefined) {
						return;
					}
					void textQuery.refetch().then(({ isError }) => {
						if (isError) {
							viewer.close();
						}
					});
				}}
			/>
			{viewer.isViewing && (
				<TextAttachmentDialog
					file={file}
					content={textQuery.data}
					{...viewer.dialogProps}
				/>
			)}
		</>
	);
};

type InlineTextAttachmentCardProps = AttachmentCardProps & { data: string };

const InlineTextAttachmentCard: React.FC<InlineTextAttachmentCardProps> = ({
	attachment: { file, href },
	data,
}) => {
	const viewer = useAttachmentViewer();

	return (
		<>
			<ViewableFileCard file={file} href={href} onView={viewer.open} />
			{viewer.isViewing && (
				<TextAttachmentDialog
					file={file}
					content={decodeInlineTextAttachment(data)}
					{...viewer.dialogProps}
				/>
			)}
		</>
	);
};

/**
 * Previews images and text files, and offers other files as downloads. For a
 * stored text file, it loads the text only on view, and a 404 marks the card
 * expired, with no view or download, while that response stays cached.
 */
export const MessageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment,
}) => {
	const { file, href } = attachment;
	if (isRasterImageMediaType(file.media_type)) {
		return <ImageAttachmentCard attachment={attachment} />;
	}
	const storedTextFileId = getStoredTextFileId(file);
	if (storedTextFileId) {
		return (
			<StoredTextAttachmentCard
				attachment={attachment}
				fileId={storedTextFileId}
			/>
		);
	}
	if (isTextPreviewAttachmentMediaType(file.media_type) && file.data) {
		return (
			<InlineTextAttachmentCard attachment={attachment} data={file.data} />
		);
	}
	return <FileCard block={file} href={href} expired={false} fullWidth />;
};
