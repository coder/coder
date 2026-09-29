import { cn } from "cn";
import { ChevronUpIcon, PencilLineIcon, XIcon } from "lucide-react";
import type { Chat } from "#/api/typesGenerated";
import { getChatDisplayConfig } from "../../components/ChatsSidebar/tree/statusConfig";
import type { CardColor } from "./boardLabels";
import { TitleBarButton } from "./ChatWindows";
import { cardAccent } from "./cardColor";
import { UnreadBadge } from "./Openers";

/** One minimized window as the tab bar shows it. */
type WindowTab = Readonly<{
	key: string;
	title: string;
	color: CardColor | undefined;
	/** Absent for a draft, which has no chat yet. */
	chat: Chat | undefined;
}>;

type WindowTabsProps = {
	readonly tabs: readonly WindowTab[];
	readonly onRestore: (key: string) => void;
	readonly onClose: (key: string) => void;
};

/** The chat's status icon, or a pencil for a draft. */
const TabIcon: React.FC<{ readonly chat: Chat | undefined }> = ({ chat }) => {
	if (!chat) {
		return (
			<PencilLineIcon
				aria-hidden="true"
				className="size-3.5 shrink-0 text-content-secondary"
			/>
		);
	}

	const display = getChatDisplayConfig(chat);
	const Icon = display.icon;

	// Unread is a dot on the chat icon, as on the board's cards.
	return (
		<span className="relative flex shrink-0">
			<Icon
				aria-label={display.label}
				className={cn("size-3.5", display.className)}
			/>
			<UnreadBadge chat={chat} />
		</span>
	);
};

type TabProps = {
	readonly tab: WindowTab;
	readonly onRestore: () => void;
	readonly onClose: () => void;
};

/** A collapsed window title bar: the tab body restores it, × closes it. */
const Tab: React.FC<TabProps> = ({ tab, onRestore, onClose }) => (
	<div
		className={cn(
			"flex h-8 w-56 shrink-0 items-center gap-2 overflow-hidden rounded-t-md border border-b-0 border-solid border-border bg-surface-primary pr-1 pl-2.5 text-[12.5px] font-medium text-content-primary transition-colors hover:bg-surface-tertiary",
			tab.color && cardAccent({ color: tab.color }),
		)}
	>
		<button
			type="button"
			aria-label={`Restore ${tab.title}`}
			className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 border-0 bg-transparent p-0 text-left text-inherit"
			onClick={onRestore}
		>
			<TabIcon chat={tab.chat} />
			<span className="min-w-0 flex-1 truncate">{tab.title}</span>
			<ChevronUpIcon
				aria-hidden="true"
				className="size-3.5 shrink-0 text-content-secondary"
			/>
		</button>

		<TitleBarButton aria-label={`Close ${tab.title}`} onClick={onClose}>
			<XIcon />
		</TitleBarButton>
	</div>
);

/**
 * Minimized windows in a strip docked under the board. The strip takes its
 * own row, so it never covers cards. Clicking a tab brings its window back
 * where it was; the window stayed mounted while hidden.
 */
export const WindowTabs: React.FC<WindowTabsProps> = ({
	tabs,
	onRestore,
	onClose,
}) => {
	if (tabs.length === 0) return null;

	return (
		<nav
			aria-label="Minimized chats"
			className="flex h-10 shrink-0 items-end justify-end gap-2 overflow-x-auto border-0 border-t border-solid border-border bg-surface-secondary px-5"
		>
			{tabs.map((tab) => (
				<Tab
					key={tab.key}
					tab={tab}
					onRestore={() => onRestore(tab.key)}
					onClose={() => onClose(tab.key)}
				/>
			))}
		</nav>
	);
};
