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
					Chat board: group chats into cards and columns, keep notes, open chats
					in floating windows, and ask an assistant to check on them, from Board
					in the sidebar. Experimental, not production ready. Groups and notes
					are saved on the chats; column order and windows live in this browser
					and reset if its storage is cleared.
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
