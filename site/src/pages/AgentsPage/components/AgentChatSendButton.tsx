import { ArrowUpIcon, CheckIcon } from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";
import type { AgentChatSendShortcut } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Kbd } from "#/components/Kbd/Kbd";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import {
	coarsePointerMediaQuery,
	mobileViewportMediaQuery,
} from "#/utils/mobile";
import { enterSendsAgentChatMessage } from "../utils/agentChatSendShortcut";

const LONG_PRESS_DELAY_MS = 500;
const LONG_PRESS_MOVE_TOLERANCE_PX = 10;

type AgentChatSendButtonProps = {
	label: string;
	sendShortcut: AgentChatSendShortcut;
	canSend: boolean;
	offersSendAfterTurn: boolean;
	isRecording: boolean;
	isSending: boolean;
	onSend: () => void;
	onSendAfterTurn: () => void;
	onAcceptRecording: () => void;
};

/**
 * Renders the chat send button. When offersSendAfterTurn and canSend are set
 * and voice input is off, a right-click or long press opens a menu to send
 * during or after the turn.
 */
export const AgentChatSendButton: React.FC<AgentChatSendButtonProps> = ({
	label,
	sendShortcut,
	canSend,
	offersSendAfterTurn,
	isRecording,
	isSending,
	onSend,
	onSendAfterTurn,
	onAcceptRecording,
}) => {
	const isMobile = useMediaQuery(mobileViewportMediaQuery);
	const isCoarsePointer = useMediaQuery(coarsePointerMediaQuery);
	const shortcuts = isMobile
		? undefined
		: enterSendsAgentChatMessage(sendShortcut, isCoarsePointer)
			? {
					send: "Enter",
					sendAfterTurn: "Cmd/Ctrl+Enter",
					ariaKeyShortcuts: "Enter",
				}
			: {
					send: "Cmd/Ctrl+Enter",
					sendAfterTurn: "Option/Alt+Enter",
					ariaKeyShortcuts: "Control+Enter Meta+Enter",
				};
	const sendActionLabel = offersSendAfterTurn ? "Send during turn" : label;

	return (
		<AgentChatSendMenu
			canOpen={offersSendAfterTurn && canSend && !isRecording}
			onSend={onSend}
			onSendAfterTurn={onSendAfterTurn}
			sendShortcutLabel={shortcuts?.send}
			sendAfterTurnShortcutLabel={shortcuts?.sendAfterTurn}
		>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						size="icon"
						variant="default"
						className="size-7 rounded-full transition-colors [&>svg]:size-5! [&>svg]:p-0"
						onClick={isRecording ? onAcceptRecording : onSend}
						disabled={isRecording ? false : !canSend}
						aria-keyshortcuts={shortcuts?.ariaKeyShortcuts}
					>
						{isSending ? (
							<Spinner size="sm" loading aria-hidden="true" />
						) : isRecording ? (
							<CheckIcon />
						) : (
							<ArrowUpIcon />
						)}
						<span className="sr-only">
							{isRecording ? "Accept voice input" : sendActionLabel}
						</span>
					</Button>
				</TooltipTrigger>
				<TooltipContent side="top">
					{isRecording ? (
						"Accept voice input"
					) : (
						<>
							{shortcuts
								? `${sendActionLabel}: ${shortcuts.send}`
								: sendActionLabel}
							{offersSendAfterTurn && shortcuts && (
								<div>Send after turn: {shortcuts.sendAfterTurn}</div>
							)}
						</>
					)}
				</TooltipContent>
			</Tooltip>
		</AgentChatSendMenu>
	);
};

type AgentChatSendMenuProps = {
	canOpen: boolean;
	onSend: () => void;
	onSendAfterTurn: () => void;
	sendShortcutLabel: string | undefined;
	sendAfterTurnShortcutLabel: string | undefined;
	children: React.ReactNode;
};

/**
 * DropdownMenu anchors the menu to the button, not the pointer.
 */
const AgentChatSendMenu: React.FC<AgentChatSendMenuProps> = ({
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
	// A selection leaves focus to the chosen action.
	const select = (action: () => void) => {
		returnFocusRef.current = null;
		action();
	};

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			{/* The trigger only anchors the menu; Send keeps its own click and keys.
			    Its span does not need Radix's button type or popup ARIA attributes. */}
			<DropdownMenuTrigger
				asChild
				disabled
				type={undefined}
				// biome-ignore lint/a11y/useValidAriaValues: undefined removes the Radix default.
				aria-haspopup={undefined}
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
					// The mouse events that end a long press would focus Send and
					// dismiss the menu the press just opened.
					onMouseDown={(event) => {
						if (suppressClickRef.current) {
							event.preventDefault();
						}
					}}
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
				// Restore focus explicitly because the trigger is a nonfocusable span.
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

type MenuShortcutProps = {
	label: string | undefined;
};

const MenuShortcut: React.FC<MenuShortcutProps> = ({ label }) =>
	label ? (
		<Kbd aria-hidden="true" className="ml-auto">
			{label}
		</Kbd>
	) : null;
