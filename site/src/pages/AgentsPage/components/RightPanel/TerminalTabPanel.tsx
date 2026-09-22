import { cn } from "cn";
import { ChevronDownIcon, PlusIcon, XIcon } from "lucide-react";
import { type FC, useEffect, useId, useRef, useState } from "react";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { TerminalPanel } from "../TerminalPanel";
import { getSubTabElementId, SubTabStrip } from "./SubTabStrip";

/** One terminal session shown as a chip inside the Terminal tab. */
export type TerminalChip = {
	id: string;
	label: string;
	reconnectionToken: string;
	initialCommand?: string;
};

type TerminalTabPanelProps = {
	chatId: string;
	workspace: Workspace;
	workspaceAgent: WorkspaceAgent;
	terminals: readonly TerminalChip[];
	activeTerminalId: string | null;
	/** A terminal that was just opened and becomes active once it reports ready. */
	pendingTerminalId: string | null;
	/** Whether the right panel is open with the Terminal tab selected. */
	isVisible: boolean;
	canCreateTerminal: boolean;
	onActiveTerminalChange: (terminalId: string) => void;
	onCloseTerminal: (terminalId: string) => void;
	onNewTerminal: () => void;
	onTerminalReady: (terminalId: string) => void;
};

/**
 * Reports whether the element's content is wider than the element, tracking
 * both container resizes and content changes identified by `contentKey`.
 */
function useOverflows(contentKey: string) {
	const ref = useRef<HTMLDivElement>(null);
	const [overflows, setOverflows] = useState(false);

	useEffect(() => {
		const element = ref.current;
		if (!element) {
			return;
		}
		const measure = () => {
			setOverflows(element.scrollWidth > element.clientWidth + 1);
		};
		measure();
		const resizeObserver = new ResizeObserver(measure);
		resizeObserver.observe(element);
		return () => resizeObserver.disconnect();
	}, [contentKey]);

	return { ref, overflows };
}

const AddTerminalButton: FC<{
	onClick: () => void;
	disabled: boolean;
}> = ({ onClick, disabled }) => (
	<Button
		variant="outline"
		size="icon"
		onClick={onClick}
		disabled={disabled}
		aria-label="New terminal"
		title="New terminal"
		className="size-7 shrink-0 p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3.5"
	>
		<PlusIcon />
	</Button>
);

type TerminalSelectorProps = Pick<
	TerminalTabPanelProps,
	| "terminals"
	| "activeTerminalId"
	| "canCreateTerminal"
	| "onActiveTerminalChange"
	| "onCloseTerminal"
	| "onNewTerminal"
>;

/** Compact form of the chip strip, used once the chips no longer fit. */
const TerminalSelector: FC<TerminalSelectorProps> = ({
	terminals,
	activeTerminalId,
	canCreateTerminal,
	onActiveTerminalChange,
	onCloseTerminal,
	onNewTerminal,
}) => {
	const [open, setOpen] = useState(false);
	const activeTerminal = terminals.find(
		(terminal) => terminal.id === activeTerminalId,
	);

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<Button
					variant="outline"
					size="sm"
					className="h-8 max-w-full min-w-0 justify-between gap-2 px-3 text-xs"
				>
					<span className="truncate">
						{activeTerminal?.label ?? "Terminals"}
					</span>
					<ChevronDownIcon
						className={cn(
							"size-3.5 shrink-0 transition-transform",
							open && "rotate-180",
						)}
					/>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				side="bottom"
				className="w-56 p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs [&_svg]:size-3.5!"
			>
				<DropdownMenuRadioGroup
					value={activeTerminalId ?? undefined}
					onValueChange={onActiveTerminalChange}
				>
					{terminals.map((terminal) => (
						<DropdownMenuRadioItem key={terminal.id} value={terminal.id}>
							{terminal.label}
						</DropdownMenuRadioItem>
					))}
				</DropdownMenuRadioGroup>
				<DropdownMenuSeparator className="my-1" />
				<DropdownMenuItem
					onSelect={onNewTerminal}
					disabled={!canCreateTerminal}
				>
					<PlusIcon />
					New terminal
				</DropdownMenuItem>
				{activeTerminal && (
					<DropdownMenuItem onSelect={() => onCloseTerminal(activeTerminal.id)}>
						<XIcon />
						Close {activeTerminal.label}
					</DropdownMenuItem>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

export const TerminalTabPanel: FC<TerminalTabPanelProps> = ({
	chatId,
	workspace,
	workspaceAgent,
	terminals,
	activeTerminalId,
	pendingTerminalId,
	isVisible,
	canCreateTerminal,
	onActiveTerminalChange,
	onCloseTerminal,
	onNewTerminal,
	onTerminalReady,
}) => {
	const idPrefix = useId();
	// A hidden copy of the chip row measures whether every chip fits on one
	// line; when it does not, the strip collapses into a selector.
	const { ref: measureRef, overflows } = useOverflows(
		terminals.map((terminal) => terminal.label).join("\n"),
	);

	if (terminals.length === 0) {
		return (
			<div className="flex h-full min-h-0 flex-col items-center justify-center gap-3 px-6 text-center">
				<Button
					variant="outline"
					size="sm"
					onClick={onNewTerminal}
					disabled={!canCreateTerminal}
				>
					<PlusIcon />
					Add a terminal
				</Button>
				{!canCreateTerminal && (
					<p className="m-0 text-xs text-content-secondary">
						Start the workspace to add a terminal.
					</p>
				)}
			</div>
		);
	}

	const chipTabs = terminals.map((terminal) => ({
		id: terminal.id,
		label: terminal.label,
		onClose: () => onCloseTerminal(terminal.id),
	}));

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="relative flex shrink-0 items-center border-0 border-b border-solid border-border-default px-3 py-2">
				<div
					ref={measureRef}
					aria-hidden
					inert
					className="invisible absolute inset-x-3 top-0 flex items-center gap-1.5 overflow-hidden"
				>
					<SubTabStrip
						label="Terminals"
						idPrefix={`${idPrefix}-measure`}
						tabs={chipTabs}
						activeTabId={activeTerminalId}
						onActiveTabChange={() => {}}
						className="w-max flex-none overflow-visible"
					/>
					<AddTerminalButton onClick={() => {}} disabled={false} />
				</div>
				{overflows ? (
					<TerminalSelector
						terminals={terminals}
						activeTerminalId={activeTerminalId}
						canCreateTerminal={canCreateTerminal}
						onActiveTerminalChange={onActiveTerminalChange}
						onCloseTerminal={onCloseTerminal}
						onNewTerminal={onNewTerminal}
					/>
				) : (
					<SubTabStrip
						label="Terminals"
						idPrefix={idPrefix}
						tabs={chipTabs}
						activeTabId={activeTerminalId}
						onActiveTabChange={onActiveTerminalChange}
						trailing={
							<AddTerminalButton
								onClick={onNewTerminal}
								disabled={!canCreateTerminal}
							/>
						}
					/>
				)}
			</div>
			<div className="relative flex min-h-0 flex-1 flex-col">
				{terminals.map((terminal) => {
					const isActive = terminal.id === activeTerminalId;
					// The selector is a menu, not a tablist, so the panels only carry
					// tab semantics while the chips are shown.
					const tabPanelProps = overflows
						? {}
						: {
								role: "tabpanel",
								"aria-labelledby": getSubTabElementId(idPrefix, terminal.id),
							};
					return (
						<div
							key={terminal.id}
							{...tabPanelProps}
							className={cn(
								"flex min-h-0 flex-1 flex-col",
								// Inactive terminals stay laid out so xterm keeps its canvas
								// and switching back does not refit or flash.
								!isActive && "invisible absolute inset-0",
							)}
							inert={!isActive}
						>
							<TerminalPanel
								chatId={chatId}
								reconnectionToken={terminal.reconnectionToken}
								initialCommand={terminal.initialCommand}
								isHot={
									isVisible && (isActive || pendingTerminalId === terminal.id)
								}
								autoFocus={isVisible && isActive}
								onReady={() => onTerminalReady(terminal.id)}
								workspace={workspace}
								workspaceAgent={workspaceAgent}
							/>
						</div>
					);
				})}
			</div>
		</div>
	);
};
