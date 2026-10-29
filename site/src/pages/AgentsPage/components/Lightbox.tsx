import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";

type LightboxProps = {
	/** Screen-reader title for the dialog. */
	title: string;
	onClose: () => void;
	/** Radix focuses the dialog's Trigger after this runs unless it calls
	 * event.preventDefault(), and a Lightbox has no Trigger, so focus would
	 * otherwise fall to the page body. */
	onCloseAutoFocus?: (event: Event) => void;
	children: React.ReactNode;
};

/**
 * Chat preview dialog for media that should be shown as large as the
 * viewport allows: a dimmed overlay with an unpadded, borderless frame
 * capped at 90vw by 85vh. Children are responsible for their own
 * surface and for fitting within those bounds.
 */
export const Lightbox: React.FC<LightboxProps> = ({
	title,
	onClose,
	onCloseAutoFocus,
	children,
}) => {
	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[85vh] max-w-[90vw] w-fit border-0 bg-transparent p-0 shadow-none"
				aria-describedby={undefined}
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<DialogTitle className="sr-only">{title}</DialogTitle>
				{children}
			</DialogContent>
		</Dialog>
	);
};
