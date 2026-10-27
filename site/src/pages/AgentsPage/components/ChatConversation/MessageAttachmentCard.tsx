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
import { useFileProbes } from "./FileProbeContext";
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
	/** Shown in place of the file type badge. */
	tile?: React.ReactNode;
	/** Shown under the name in place of "View file". */
	status?: string | null;
	ref?: React.RefCallback<HTMLElement>;
} & (
	| {
			onView: (event: React.MouseEvent<HTMLButtonElement>) => void;
			/**
			 * The file is gone: the button keeps focus but no longer views or
			 * downloads it.
			 */
			expired?: boolean;
	  }
	| { onView?: undefined; expired: true }
);

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

type ExpirableAttachmentCardProps = AttachmentCardProps & { expired: boolean };

/**
 * A ref for every card a component swaps between, so when a swap removes the
 * focused card, the new card takes focus instead of the page body. An open
 * viewer holds focus in its portal, so `isViewing` counts as focus.
 */
const useSwapFocusRef = (isViewing = false) => {
	const hadFocusRef = useRef(false);
	return (node: HTMLElement) => {
		if (hadFocusRef.current && document.activeElement === document.body) {
			// The expired card's ref is its wrapper, which holds its button.
			(node.querySelector("button") ?? node).focus();
		}
		// React runs ref cleanups before it removes the node, so focus inside
		// it has not fallen to the page body yet.
		return () => {
			hadFocusRef.current = isViewing || node.contains(document.activeElement);
		};
	};
};

/**
 * A thumbnail that fails to load swaps the card for a download card, since
 * the viewer would show the same broken image.
 */
const ImageAttachmentCard: React.FC<ExpirableAttachmentCardProps> = ({
	attachment: { file, href },
	expired,
}) => {
	const [hasFailed, setHasFailed] = useState(false);
	const viewer = useAttachmentViewer();
	const swapFocusRef = useSwapFocusRef(viewer.isViewing);

	if (expired) {
		return (
			<ViewableFileCard file={file} href={href} expired ref={swapFocusRef} />
		);
	}
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
	const { hasExpired, markExpired } = useFileProbes();
	const textQuery = useQuery(chatFileText(fileId));
	const loadError = textQuery.isError
		? getTextAttachmentErrorMessage(textQuery.error)
		: null;

	return (
		<>
			<ViewableFileCard
				file={file}
				href={href}
				expired={hasExpired(fileId) || isExpiredFileError(textQuery.error)}
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
					void textQuery.refetch().then(({ isError, error }) => {
						if (isError) {
							viewer.close();
						}
						if (isExpiredFileError(error)) {
							markExpired(fileId);
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

const DownloadFileCard: React.FC<ExpirableAttachmentCardProps> = ({
	attachment: { file, href },
	expired,
}) => {
	const swapFocusRef = useSwapFocusRef();
	if (expired) {
		return (
			<ViewableFileCard file={file} href={href} expired ref={swapFocusRef} />
		);
	}
	return (
		<FileCard
			block={file}
			href={href}
			expired={false}
			fullWidth
			ref={swapFocusRef}
		/>
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
 * Previews images and text files, and offers other files as downloads. A
 * stored text file loads its text only on view. Expired files get an expired
 * card with no view or download, and inside a FileProbeProvider a stored text
 * file's 404 marks the file expired for the whole timeline.
 */
export const MessageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment,
}) => {
	const { file } = attachment;
	const { hasExpired } = useFileProbes();
	const isKnownExpired = hasExpired(file.file_id);

	// Cards take expiry as a prop, not an early return here, so a focused card
	// keeps focus when its file expires.
	if (isRasterImageMediaType(file.media_type)) {
		return (
			<ImageAttachmentCard attachment={attachment} expired={isKnownExpired} />
		);
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

	return <DownloadFileCard attachment={attachment} expired={isKnownExpired} />;
};
