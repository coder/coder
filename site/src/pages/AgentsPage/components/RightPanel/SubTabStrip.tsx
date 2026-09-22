import { cn } from "cn";
import { XIcon } from "lucide-react";
import type { FC, KeyboardEvent, ReactNode } from "react";
import { Button } from "#/components/Button/Button";

/** A chip inside a group tab, such as one terminal or one app preview. */
export type SubTab = {
	id: string;
	label: string;
	icon?: ReactNode;
	onClose?: () => void;
};

type SubTabStripProps = {
	tabs: readonly SubTab[];
	activeTabId: string | null;
	onActiveTabChange: (tabId: string) => void;
	/** Accessible name for the tablist, for example "Terminals". */
	label: string;
	/**
	 * Prefix for chip element IDs so a parent can point its tab panels at
	 * them with `aria-labelledby`. Build IDs with `getSubTabElementId`.
	 */
	idPrefix: string;
	/** Rendered after the last chip, typically the add control. */
	trailing?: ReactNode;
	className?: string;
};

export function getSubTabElementId(idPrefix: string, tabId: string): string {
	return `${idPrefix}-chip-${tabId}`;
}

const chipActiveClassName =
	"bg-surface-quaternary/25 text-content-primary hover:bg-surface-quaternary/50";

/**
 * Horizontal strip of closeable chips with roving focus: arrow keys move
 * between chips and select them, matching the WAI-ARIA tabs pattern.
 */
export const SubTabStrip: FC<SubTabStripProps> = ({
	tabs,
	activeTabId,
	onActiveTabChange,
	label,
	idPrefix,
	trailing,
	className,
}) => {
	const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
		if (tabs.length === 0) {
			return;
		}
		const currentIndex = tabs.findIndex((tab) => tab.id === activeTabId);
		let nextIndex: number;
		switch (event.key) {
			case "ArrowRight":
				nextIndex = (currentIndex + 1) % tabs.length;
				break;
			case "ArrowLeft":
				nextIndex = (currentIndex - 1 + tabs.length) % tabs.length;
				break;
			case "Home":
				nextIndex = 0;
				break;
			case "End":
				nextIndex = tabs.length - 1;
				break;
			default:
				return;
		}
		event.preventDefault();
		const nextTab = tabs[nextIndex];
		onActiveTabChange(nextTab.id);
		document.getElementById(getSubTabElementId(idPrefix, nextTab.id))?.focus();
	};

	return (
		<div
			role="tablist"
			aria-label={label}
			aria-orientation="horizontal"
			onKeyDown={handleKeyDown}
			className={cn(
				"flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto scrollbar-none [&::-webkit-scrollbar]:hidden",
				className,
			)}
		>
			{tabs.map((tab) => {
				const isActive = tab.id === activeTabId;
				return (
					<div
						key={tab.id}
						className="group flex shrink-0 items-stretch rounded-md border border-solid border-border-default bg-surface-primary"
					>
						<Button
							id={getSubTabElementId(idPrefix, tab.id)}
							role="tab"
							aria-selected={isActive}
							tabIndex={isActive ? 0 : -1}
							onClick={() => onActiveTabChange(tab.id)}
							variant="subtle"
							size="sm"
							className={cn(
								"h-7 min-w-0 gap-1.5 rounded-md border-0 px-2.5 text-xs text-content-secondary hover:text-content-primary",
								tab.onClose && "rounded-r-none pr-1.5",
								isActive && chipActiveClassName,
							)}
						>
							{tab.icon}
							<span className="truncate">{tab.label}</span>
						</Button>
						{tab.onClose && (
							<Button
								variant="subtle"
								size="icon"
								onClick={tab.onClose}
								aria-label={`Close ${tab.label}`}
								className={cn(
									"h-7 w-6 rounded-l-none rounded-r-md border-0 p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3",
									isActive && chipActiveClassName,
								)}
							>
								<XIcon />
							</Button>
						)}
					</div>
				);
			})}
			{trailing}
		</div>
	);
};
