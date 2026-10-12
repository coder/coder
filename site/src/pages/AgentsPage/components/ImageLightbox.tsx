import { Lightbox } from "./Lightbox";

type ImageLightboxProps = {
	src: string;
	onClose: () => void;
};

export const ImageLightbox: React.FC<ImageLightboxProps> = ({
	src,
	onClose,
}) => {
	return (
		<Lightbox title="Image preview" onClose={onClose}>
			<img
				src={src}
				alt="Attachment preview"
				className="max-h-[85vh] max-w-[90vw] rounded object-contain"
			/>
		</Lightbox>
	);
};
