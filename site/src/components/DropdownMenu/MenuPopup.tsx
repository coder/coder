import { Menu } from "@base-ui/react/menu";

type MenuPopupProps = Omit<Menu.Popup.Props, "className" | "onSelect"> & {
	className?: string;
	onCloseAutoFocus?: (event: Event) => void;
};

/** Preserves cancelable return-focus callbacks for dropdown and context menus. */
export const MenuPopup: React.FC<MenuPopupProps> = ({
	onCloseAutoFocus,
	...props
}) => (
	<Menu.Popup
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
