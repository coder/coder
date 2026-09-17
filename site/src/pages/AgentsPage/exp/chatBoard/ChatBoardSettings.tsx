import type { FC } from "react";
import { Switch } from "#/components/Switch/Switch";
import { saveChatBoardEnabled, useChatBoardEnabled } from "./chatBoardFlag";

/** Opt-in switch for the chat board, listed under Experiments in settings. */
export const ChatBoardSettings: FC = () => {
	const enabled = useChatBoardEnabled();

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Experiments
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 flex-1 text-xs text-content-secondary">
					Chat board: a kanban view of your chats with cards, notes and floating
					chat windows. Adds a Board entry to the sidebar.
				</p>
				<Switch
					checked={enabled}
					onCheckedChange={(checked) => saveChatBoardEnabled(Boolean(checked))}
					aria-label="Chat board"
				/>
			</div>
		</div>
	);
};
