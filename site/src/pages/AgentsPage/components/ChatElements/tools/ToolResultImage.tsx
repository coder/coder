import { ImageLightbox } from "../../ImageLightbox";
import { useLightbox } from "../../LightboxHost";

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
					className="cursor-pointer bg-transparent p-0 border-none"
					onClick={() =>
						openLightbox((close) => (
							<ImageLightbox src={imageSrc} onClose={close} />
						))
					}
				>
					<img
						src={imageSrc}
						alt={alt}
						className="max-h-96 w-auto object-contain"
					/>
				</button>
			</div>
			{lightbox}
		</>
	);
};
