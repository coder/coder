import { ToolResultImage } from "./ToolResultImage";
import type { MediaToolResult } from "./utils";

export const ToolResultMedia: React.FC<{ media: MediaToolResult }> = ({
	media,
}) => (
	<>
		{media.mimeType.startsWith("image/") ? (
			<ToolResultImage
				data={media.data}
				mimeType={media.mimeType}
				alt="Image from tool result"
			/>
		) : (
			<div className="mt-1.5 text-xs text-content-secondary">
				{media.mimeType} content is not previewable.
			</div>
		)}
		{media.text && (
			<pre className="mt-1.5 whitespace-pre-wrap break-words text-xs text-content-secondary">
				{media.text}
			</pre>
		)}
	</>
);
