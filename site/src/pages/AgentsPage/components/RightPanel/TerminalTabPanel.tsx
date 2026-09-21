import { cn } from "cn";
import { PlusIcon, SquareTerminalIcon } from "lucide-react";
import { type FC, useId } from "react";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { TerminalPanel } from "../TerminalPanel";
import { RightPanelEmptyState } from "./RightPanelEmptyState";
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

	if (terminals.length === 0) {
		return (
			<RightPanelEmptyState
				icon={<SquareTerminalIcon />}
				title="No terminals open"
				description={
					canCreateTerminal
						? "Open a terminal to run commands in the workspace."
						: "Start the workspace to open a terminal."
				}
				action={
					<Button
						variant="outline"
						size="sm"
						onClick={onNewTerminal}
						disabled={!canCreateTerminal}
					>
						<PlusIcon />
						New terminal
					</Button>
				}
			/>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<SubTabStrip
				label="Terminals"
				idPrefix={idPrefix}
				tabs={terminals.map((terminal) => ({
					id: terminal.id,
					label: terminal.label,
					onClose: () => onCloseTerminal(terminal.id),
				}))}
				activeTabId={activeTerminalId}
				onActiveTabChange={onActiveTerminalChange}
				trailing={
					<Button
						variant="outline"
						size="icon"
						onClick={onNewTerminal}
						disabled={!canCreateTerminal}
						aria-label="New terminal"
						title="New terminal"
						className="size-7 shrink-0 p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3.5"
					>
						<PlusIcon />
					</Button>
				}
			/>
			<div className="relative flex min-h-0 flex-1 flex-col">
				{terminals.map((terminal) => {
					const isActive = terminal.id === activeTerminalId;
					return (
						<div
							key={terminal.id}
							role="tabpanel"
							aria-labelledby={getSubTabElementId(idPrefix, terminal.id)}
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
