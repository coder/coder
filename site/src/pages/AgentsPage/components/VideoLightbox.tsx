import { useState } from "react";
import { RECORDING_UNAVAILABLE_TEXT } from "./ChatElements/tools/previewConstants";
import { Lightbox } from "./Lightbox";

type VideoLightboxProps = {
	src: string;
	onClose: () => void;
};

export const VideoLightbox: React.FC<VideoLightboxProps> = ({
	src,
	onClose,
}) => {
	const [videoError, setVideoError] = useState(false);

	return (
		<Lightbox title="Recording playback" onClose={onClose}>
			{videoError ? (
				<div className="flex items-center justify-center rounded bg-surface-secondary p-8 text-sm text-content-secondary">
					{RECORDING_UNAVAILABLE_TEXT}
				</div>
			) : (
				// oxlint-disable-next-line jsx-a11y/media-has-caption -- Screen recordings do not have caption tracks.
				<video
					src={src}
					controls
					className="max-h-[85vh] max-w-[90vw] rounded"
					onError={() => setVideoError(true)}
				/>
			)}
		</Lightbox>
	);
};
