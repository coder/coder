import { isAxiosError } from "axios";
import { useRef, useState } from "react";
import { useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { chatFileText } from "#/api/queries/chats";
import { Spinner } from "#/components/Spinner/Spinner";
import { isRasterImageMediaType } from "../../utils/chatAttachments";
import { decodeInlineTextAttachment } from "../../utils/fetchTextAttachment";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import { useDialogFocusReturn } from "../useDialogFocusReturn";
import {
	FileCard,
	getAttachmentDisplayName,
	isTextPreviewAttachmentMediaType,
	ViewableFileCard,
} from "./AttachmentBlocks";
import { useFileProbes } from "./FileProbeContext";
import type { AgentAttachment } from "./messageHelpers";
import { useAttachmentFailure } from "./useAttachmentFailure";

type AttachmentViewer = ReturnType<ReturnType<typeof useAttachmentViewer>>;

type AttachmentCardProps = {
	attachment: AgentAttachment;
	viewer: AttachmentViewer;
};

/**
 * One viewer per list, so a slow read that finishes after another file was
 * viewed cannot open a second preview.
 */
const useAttachmentViewer = () => {
	const [viewedKey, setViewedKey] = useState<string>();
	const { rememberTrigger, restoreFocus } = useDialogFocusReturn();
	return (key: string) => {
		const close = () =>
			setViewedKey((current) => (current === key ? undefined : current));
		return {
			isViewing: viewedKey === key,
			open: (event: React.MouseEvent<HTMLButtonElement>) => {
				rememberTrigger(event);
				setViewedKey(key);
			},
			close,
			dialogProps: { onClose: close, onCloseAutoFocus: restoreFocus },
		};
	};
};

/**
 * A ref for each root a card swaps between, so that when a failure swaps out
 * the root holding focus, its replacement takes focus, not the page body.
 */
const useSwapFocusRef = () => {
	const hadFocusRef = useRef(false);
	return (node: HTMLElement) => {
		if (hadFocusRef.current && document.activeElement === document.body) {
			node.focus();
		}
		// React runs ref cleanups before it removes the node, so focus inside
		// it has not fallen to the page body yet.
		return () => {
			hadFocusRef.current = node.contains(document.activeElement);
		};
	};
};

const ImageAttachmentCard: React.FC<AttachmentCardProps> = ({
	attachment: { file, href },
	viewer,
}) => {
	const [failure, onError] = useAttachmentFailure(file.file_id, href);
	const swapFocusRef = useSwapFocusRef();

	// The viewer would show the same broken image as a failed thumbnail.
	if (failure.kind !== "idle") {
		return <FileCard block={file} href={href} fullWidth ref={swapFocusRef} />;
	}

	return (
		<>
			<ViewableFileCard
				block={file}
				href={href}
				tile={
					<img
						src={href}
						alt=""
						className="size-full object-cover"
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

const isExpiredFileError = (error: unknown) =>
	isAxiosError(error) && error.response?.status === 404;

const StoredTextAttachmentCard: React.FC<StoredTextAttachmentCardProps> = ({
	attachment: { file, href },
	fileId,
	viewer,
}) => {
	const swapFocusRef = useSwapFocusRef();
	const { hasExpired, markExpired } = useFileProbes();
	// Only the view handler fetches, so that it can share a 404 with the
	// timeline's other views of the file.
	const textQuery = useQuery({ ...chatFileText(fileId), enabled: false });
	const isLoadingText = textQuery.isFetching && textQuery.data === undefined;
	const loadError = textQuery.isError
		? getErrorMessage(
				textQuery.error,
				"Couldn't load preview. Select again to retry.",
			)
		: undefined;

	if (hasExpired(fileId)) {
		return <FileCard block={file} href={href} fullWidth ref={swapFocusRef} />;
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
					if (textQuery.data !== undefined) {
						return;
					}
					void textQuery.refetch().then(({ error, isError }) => {
						if (!isError) {
							return;
						}
						// Left open, this card's preview would show once another
						// card loads the file.
						viewer.close();
						if (isExpiredFileError(error)) {
							markExpired(fileId);
						}
					});
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
	viewer: AttachmentViewer;
};

const InlineTextAttachmentCard: React.FC<InlineTextAttachmentCardProps> = ({
	attachment: { file, href },
	data,
	viewer,
}) => {
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
					viewer={props.viewer}
				/>
			);
		}
	}
	return <FileCard block={file} href={href} fullWidth />;
};

type AgentAttachmentListProps = {
	attachments: readonly AgentAttachment[];
};

/** A card for each attachment, which views or downloads its file. */
export const AgentAttachmentList: React.FC<AgentAttachmentListProps> = ({
	attachments,
}) => {
	const viewerFor = useAttachmentViewer();
	return (
		<ul className="flex flex-col gap-2">
			{attachments.map((attachment) => (
				<li key={attachment.key}>
					<AgentAttachmentCard
						attachment={attachment}
						viewer={viewerFor(attachment.key)}
					/>
				</li>
			))}
		</ul>
	);
};
