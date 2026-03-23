import { Dialog, DialogContent, DialogTitle } from "components/Dialog/Dialog";
import type { FC } from "react";

interface ImageLightboxProps {
	src: string;
	onClose: () => void;
}

export const ImageLightbox: FC<ImageLightboxProps> = ({ src, onClose }) => {
	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[85vh] max-w-[90vw] w-fit border-0 bg-transparent p-0 shadow-none"
				aria-describedby={undefined}
			>
				<DialogTitle className="sr-only">Image preview</DialogTitle>
				<img
					src={src}
					alt="Attachment preview"
					className="max-h-[85vh] max-w-[90vw] rounded object-contain"
				/>
			</DialogContent>
		</Dialog>
	);
};
