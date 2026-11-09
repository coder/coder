import { Dialog } from "@base-ui/react/dialog";

export type DialogPopupProps = Omit<Dialog.Popup.Props, "className"> & {
	className?: string;
	onOpenAutoFocus?: (event: Event) => void;
	onCloseAutoFocus?: (event: Event) => void;
};

/** Preserves the focus callbacks shared by dialogs and drawers. */
export const DialogPopup: React.FC<DialogPopupProps> = ({
	onOpenAutoFocus,
	onCloseAutoFocus,
	...props
}) => (
	<Dialog.Popup
		initialFocus={
			onOpenAutoFocus
				? () => {
						const event = new Event("openAutoFocus", { cancelable: true });
						onOpenAutoFocus(event);
						return !event.defaultPrevented;
					}
				: undefined
		}
		finalFocus={
			onCloseAutoFocus
				? () => {
						const event = new Event("closeAutoFocus", { cancelable: true });
						onCloseAutoFocus(event);
						return !event.defaultPrevented;
					}
				: undefined
		}
		{...props}
	/>
);
