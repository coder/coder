import type React from "react";
import { useRef, useState } from "react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Kbd } from "#/components/Kbd/Kbd";

const LONG_PRESS_DELAY_MS = 500;
// A press that moves farther than this is a scroll, not a long press.
const LONG_PRESS_MOVE_TOLERANCE_PX = 10;

type AgentChatSendMenuProps = {
	/**
	 * Whether a right-click or a long press opens the menu. When false, the
	 * browser's native context menu stays.
	 */
	canOpen: boolean;
	onSend: () => void;
	onSendAfterTurn: () => void;
	sendShortcutLabel: string | undefined;
	sendAfterTurnShortcutLabel: string | undefined;
	children: React.ReactNode;
};

/**
 * Wraps the send button with a menu that chooses when a busy agent receives
 * the message. A right-click, or a long press on a touch screen, opens it.
 */
export const AgentChatSendMenu: React.FC<AgentChatSendMenuProps> = ({
	canOpen,
	onSend,
	onSendAfterTurn,
	sendShortcutLabel,
	sendAfterTurnShortcutLabel,
	children,
}) => {
	const [open, setOpen] = useState(false);
	const pressRef = useRef<{ timer: number; x: number; y: number } | null>(null);
	// The click that ends a long press must not also send.
	const suppressClickRef = useRef(false);
	const returnFocusRef = useRef<HTMLElement | null>(null);

	// The menu offers "Send after turn", so it closes when the turn ends.
	if (open && !canOpen) {
		setOpen(false);
	}

	const cancelPress = () => {
		if (pressRef.current) {
			window.clearTimeout(pressRef.current.timer);
			pressRef.current = null;
		}
	};
	const openMenu = () => {
		returnFocusRef.current =
			document.activeElement instanceof HTMLElement
				? document.activeElement
				: null;
		setOpen(true);
	};
	// The disabled trigger takes no focus, so a dismissal restores focus by
	// hand. A selection leaves focus to the chosen action.
	const select = (action: () => void) => {
		returnFocusRef.current = null;
		action();
	};

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			{/* The send button keeps its own click and keys, so this disabled
			    trigger only anchors the menu. A span cannot carry the button
			    type or aria-expanded. */}
			<DropdownMenuTrigger
				asChild
				disabled
				type={undefined}
				aria-expanded={undefined}
			>
				<span
					className="inline-flex select-none [-webkit-touch-callout:none]"
					onPointerDown={(event) => {
						suppressClickRef.current = false;
						cancelPress();
						if (!canOpen || event.pointerType === "mouse") {
							return;
						}
						pressRef.current = {
							x: event.clientX,
							y: event.clientY,
							timer: window.setTimeout(() => {
								pressRef.current = null;
								suppressClickRef.current = true;
								openMenu();
							}, LONG_PRESS_DELAY_MS),
						};
					}}
					onPointerMove={(event) => {
						const press = pressRef.current;
						if (
							press &&
							Math.hypot(event.clientX - press.x, event.clientY - press.y) >
								LONG_PRESS_MOVE_TOLERANCE_PX
						) {
							cancelPress();
						}
					}}
					onPointerUp={cancelPress}
					onPointerCancel={cancelPress}
					onClickCapture={(event) => {
						if (suppressClickRef.current) {
							suppressClickRef.current = false;
							event.preventDefault();
							event.stopPropagation();
						}
					}}
					onContextMenu={(event) => {
						if (!canOpen) {
							return;
						}
						event.preventDefault();
						// Touch browsers that fire contextmenu on a long press
						// end the press here.
						if (pressRef.current) {
							cancelPress();
							suppressClickRef.current = true;
						}
						if (!open) {
							openMenu();
						}
					}}
				>
					{children}
				</span>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				side="top"
				align="end"
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					returnFocusRef.current?.focus();
					returnFocusRef.current = null;
				}}
			>
				<DropdownMenuItem onSelect={() => select(onSend)}>
					Send during turn
					<MenuShortcut label={sendShortcutLabel} />
				</DropdownMenuItem>
				<DropdownMenuItem onSelect={() => select(onSendAfterTurn)}>
					Send after turn
					<MenuShortcut label={sendAfterTurnShortcutLabel} />
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const MenuShortcut: React.FC<{ label: string | undefined }> = ({ label }) =>
	label ? (
		<Kbd aria-hidden="true" className="ml-auto">
			{label}
		</Kbd>
	) : null;
