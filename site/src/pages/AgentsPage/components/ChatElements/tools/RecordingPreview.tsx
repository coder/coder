import {
	ImageOffIcon,
	type LucideIcon,
	PlayIcon,
	VideoOffIcon,
} from "lucide-react";
import type React from "react";
import { useState } from "react";
import { getChatFileURL } from "../../../utils/chatAttachments";
import { useFileProbes } from "../../ChatConversation/FileProbeContext";
import { VideoLightbox } from "../../VideoLightbox";
import {
	DEFAULT_ASPECT,
	PREVIEW_HEIGHT,
	RECORDING_EXPIRED_TEXT,
} from "./previewConstants";

const frameClassName =
	"relative overflow-hidden rounded-lg border border-solid border-border-default";
const frameStyle = { aspectRatio: DEFAULT_ASPECT, height: PREVIEW_HEIGHT };

const PreviewNotice: React.FC<{ icon: LucideIcon; children: string }> = ({
	icon: Icon,
	children,
}) => (
	<div className="flex size-full items-center justify-center gap-1.5 bg-surface-secondary text-xs text-content-secondary">
		<Icon className="size-3" />
		{children}
	</div>
);

interface RecordingPreviewProps {
	/** The chat file ID for the MP4 recording. */
	recordingFileId: string;
	/** File ID for the JPEG thumbnail of a completed recording. */
	thumbnailFileId?: string;
	/** Optional video URL override. When provided, this is used
	 * directly instead of deriving the URL from recordingFileId. */
	src?: string;
	/** Optional thumbnail URL override. When provided, this is used
	 * directly instead of deriving the URL from thumbnailFileId. */
	thumbnailSrc?: string;
}

/**
 * Inline recording thumbnail with a play icon overlay. Clicking the
 * preview opens a full-screen VideoLightbox with native playback
 * controls. If the thumbnail fails to load, a "Thumbnail unavailable"
 * message is shown but the video remains playable. A recording the chat
 * has evicted renders as an expired notice with no playback control.
 */
export const RecordingPreview: React.FC<RecordingPreviewProps> = ({
	recordingFileId,
	thumbnailFileId,
	src: srcOverride,
	thumbnailSrc: thumbnailSrcOverride,
}) => {
	const { hasExpired } = useFileProbes();
	const [showLightbox, setShowLightbox] = useState(false);
	const [thumbnailError, setThumbnailError] = useState(false);
	// Incremented each time the lightbox opens so the VideoLightbox
	// component remounts and resets its internal error state.
	const [lightboxKey, setLightboxKey] = useState(0);

	if (hasExpired(recordingFileId)) {
		return (
			<div className={frameClassName} style={frameStyle}>
				<PreviewNotice icon={VideoOffIcon}>
					{RECORDING_EXPIRED_TEXT}
				</PreviewNotice>
			</div>
		);
	}

	const videoSrc = srcOverride ?? getChatFileURL(recordingFileId);

	return (
		<div className={frameClassName} style={frameStyle}>
			{thumbnailError ? (
				<PreviewNotice icon={ImageOffIcon}>Thumbnail unavailable</PreviewNotice>
			) : thumbnailFileId ? (
				<img
					src={thumbnailSrcOverride ?? getChatFileURL(thumbnailFileId)}
					alt="Recording thumbnail"
					className="size-full pointer-events-none object-cover"
					onError={() => setThumbnailError(true)}
				/>
			) : (
				// No thumbnail available — neutral gray placeholder.
				<div className="size-full bg-surface-secondary" />
			)}
			<button
				type="button"
				aria-label="View recording"
				onClick={() => {
					setShowLightbox(true);
					setLightboxKey((k) => k + 1);
				}}
				className="absolute inset-0 z-10 flex cursor-pointer items-center justify-center border-0 bg-black/0 p-0 transition-colors hover:bg-black/50"
			>
				<span className="flex size-10 items-center justify-center rounded-full bg-black/60">
					<PlayIcon className="size-5 text-white" />
				</span>
			</button>
			<VideoLightbox
				key={lightboxKey}
				src={videoSrc}
				open={showLightbox}
				onClose={() => setShowLightbox(false)}
			/>
		</div>
	);
};
