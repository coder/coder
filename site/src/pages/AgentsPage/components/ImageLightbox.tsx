import { Lightbox } from "./Lightbox";

type ImageLightboxProps = {
	src: string;
	onClose: () => void;
	onCloseAutoFocus?: (event: Event) => void;
};

export const ImageLightbox: React.FC<ImageLightboxProps> = ({
	src,
	onClose,
	onCloseAutoFocus,
}) => {
	return (
		<Lightbox
			title="Image preview"
			onClose={onClose}
			onCloseAutoFocus={onCloseAutoFocus}
		>
			<img
				src={src}
				alt="Attachment preview"
				className="max-h-[85vh] max-w-[90vw] rounded object-contain"
			/>
		</Lightbox>
	);
};
