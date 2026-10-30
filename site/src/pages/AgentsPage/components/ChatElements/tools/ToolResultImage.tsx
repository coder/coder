import { ImageLightbox } from "../../ImageLightbox";
import { LightboxSlot, useLightbox } from "../../LightboxHost";
import { returnFocusTo } from "../../useDialogFocusReturn";

export const ToolResultImage: React.FC<{
	data: string;
	mimeType: string;
	alt: string;
}> = ({ data, mimeType, alt }) => {
	const { openLightbox, lightbox } = useLightbox();
	const imageSrc = `data:${mimeType};base64,${data}`;

	return (
		<>
			<div className="mt-1.5 overflow-hidden rounded-md border border-solid border-border">
				<button
					type="button"
					className="cursor-pointer bg-transparent p-0 border-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-solid focus-visible:outline-content-link"
					onClick={(event) => {
						const onCloseAutoFocus = returnFocusTo(event.currentTarget);
						openLightbox((close) => (
							<ImageLightbox
								src={imageSrc}
								onClose={close}
								onCloseAutoFocus={onCloseAutoFocus}
							/>
						));
					}}
				>
					<img
						src={imageSrc}
						alt={alt}
						className="max-h-96 w-auto object-contain"
					/>
				</button>
			</div>
			<LightboxSlot lightbox={lightbox} />
		</>
	);
};
