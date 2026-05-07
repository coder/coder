import { type FC, useId } from "react";
import { Switch } from "#/components/Switch/Switch";
import { useAgentChatSendShortcut } from "../hooks/useAgentChatSendShortcut";

interface ChatSendShortcutSettingsProps {
	userId: string;
}

export const ChatSendShortcutSettings: FC<ChatSendShortcutSettingsProps> = ({
	userId,
}) => {
	const [shortcut, setShortcut] = useAgentChatSendShortcut(userId);
	const descriptionId = useId();
	const requiresModifierEnter = shortcut === "modifier-enter";

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Keyboard Shortcuts
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p
					id={descriptionId}
					className="m-0 flex-1 text-xs text-content-secondary"
				>
					Require Cmd/Ctrl+Enter to send agent messages. When enabled, Enter
					inserts a newline instead.
				</p>
				<Switch
					checked={requiresModifierEnter}
					onCheckedChange={(checked) =>
						setShortcut(checked ? "modifier-enter" : "enter")
					}
					aria-label="Require Cmd/Ctrl+Enter to send messages"
					aria-describedby={descriptionId}
				/>
			</div>
		</div>
	);
};
