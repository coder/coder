import { ImageOffIcon, PlayIcon } from "lucide-react";
import type React from "react";
import { useState } from "react";
import { VideoLightbox } from "../../VideoLightbox";
import { DEFAULT_ASPECT, PREVIEW_HEIGHT } from "./previewConstants";

interface RecordingPreviewProps {
	/** The chat file ID for the MP4 recording. */
	recordingFileId: string;
	/** Optional video URL override. When provided, this is used
	 * directly instead of deriving the URL from recordingFileId. */
	src?: string;
}

/**
 * Inline recording thumbnail with a play icon overlay. Clicking the
 * preview opens a full-screen VideoLightbox with native playback
 * controls. If the thumbnail fails to load, a "Thumbnail unavailable"
 * message is shown but the video remains playable.
 */
export const RecordingPreview: React.FC<RecordingPreviewProps> = ({
	recordingFileId,
	src: srcOverride,
}) => {
	const [showLightbox, setShowLightbox] = useState(false);
	const [thumbnailError, setThumbnailError] = useState(false);
	// Incremented each time the lightbox opens so the VideoLightbox
	// component remounts and resets its internal error state.
	const [lightboxKey, setLightboxKey] = useState(0);

	const thumbnailSrc =
		// Seek to first frame so the browser renders a thumbnail preview.
		srcOverride ?? `/api/experimental/chats/files/${recordingFileId}#t=0.001`;
	const videoSrc =
		srcOverride ?? `/api/experimental/chats/files/${recordingFileId}`;

	return (
		<div
			className="relative overflow-hidden rounded-lg border border-solid border-border-default"
			style={{ aspectRatio: DEFAULT_ASPECT, height: PREVIEW_HEIGHT }}
		>
			{thumbnailError ? (
				<div className="flex h-full w-full items-center justify-center gap-1.5 bg-surface-secondary text-xs text-content-secondary">
					<ImageOffIcon className="h-3 w-3" />
					Thumbnail unavailable
				</div>
			) : (
				<video
					src={thumbnailSrc}
					preload="metadata"
					muted
					className="h-full w-full pointer-events-none object-cover"
					onError={() => setThumbnailError(true)}
				/>
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
				<span className="flex h-10 w-10 items-center justify-center rounded-full bg-black/60">
					<PlayIcon className="h-5 w-5 text-white" />
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
