import { useState } from "react";
import { ImageLightbox } from "../../ImageLightbox";
import { useDialogFocusReturn } from "../../useDialogFocusReturn";

export const ToolResultImage: React.FC<{
	data: string;
	mimeType: string;
	alt: string;
}> = ({ data, mimeType, alt }) => {
	const [showLightbox, setShowLightbox] = useState(false);
	const { rememberTrigger, restoreFocus } = useDialogFocusReturn();
	const imageSrc = `data:${mimeType};base64,${data}`;

	return (
		<>
			<div className="mt-1.5 overflow-hidden rounded-md border border-solid border-border">
				<button
					type="button"
					className="cursor-pointer bg-transparent p-0 border-none"
					onClick={(event) => {
						rememberTrigger(event);
						setShowLightbox(true);
					}}
				>
					<img
						src={imageSrc}
						alt={alt}
						className="max-h-96 w-auto object-contain"
					/>
				</button>
			</div>
			{showLightbox && (
				<ImageLightbox
					src={imageSrc}
					onClose={() => setShowLightbox(false)}
					onCloseAutoFocus={restoreFocus}
				/>
			)}
		</>
	);
};
