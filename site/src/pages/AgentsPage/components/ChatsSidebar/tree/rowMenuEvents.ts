const stopEvent = (event: React.SyntheticEvent) => {
	event.preventDefault();
	event.stopPropagation();
};

const stopSecondaryButton = (event: React.MouseEvent) => {
	if (event.button === 2) {
		stopEvent(event);
	}
};

/**
 * Spread onto a row's actions button so a right-click on it does not also
 * open the row's context menu.
 */
export const rowActionsTriggerProps = {
	onContextMenuCapture: stopEvent,
	onMouseDownCapture: stopSecondaryButton,
	onPointerDownCapture: stopSecondaryButton,
};

/**
 * The actions dropdown is portaled to the body, but React portals bubble
 * events through the React tree, so a right-click inside the menu would still
 * reach the row's context-menu trigger and open a duplicate menu.
 */
export const stopRowContextMenu = stopEvent;
