import { RotateCcwIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import { ChatTopBar } from "./components/ChatTopBar";

interface AgentChatPageErrorViewProps {
	titleElement: ReactNode;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	error: unknown;
	onRetry: () => void;
}

export const AgentChatPageErrorView: FC<AgentChatPageErrorViewProps> = ({
	titleElement,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	error,
	onRetry,
}) => {
	const detail = getErrorDetail(error);

	return (
		<div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{titleElement}
			<ChatTopBar
				panel={{
					showSidebarPanel: false,
					onToggleSidebar: () => {},
				}}
				onArchiveAgent={() => {}}
				onUnarchiveAgent={() => {}}
				onArchiveAndDeleteWorkspace={() => {}}
				hasWorkspace={false}
				isSidebarCollapsed={isSidebarCollapsed}
				onToggleSidebarCollapsed={onToggleSidebarCollapsed}
			/>
			<div className="flex flex-1 items-center justify-center px-6 text-center">
				<div className="flex flex-col items-center">
					<h3 className="m-0 font-medium text-base text-content-primary">
						Failed to load chat
					</h3>
					<p className="m-0 mt-1 max-w-md text-sm text-content-secondary">
						{getErrorMessage(error, "The chat could not be loaded.")}
					</p>
					{detail && (
						<p className="m-0 mt-1 max-w-md text-sm text-content-secondary">
							{detail}
						</p>
					)}
					<Button size="sm" onClick={onRetry} className="mt-4">
						<RotateCcwIcon />
						Try again
					</Button>
				</div>
			</div>
		</div>
	);
};
