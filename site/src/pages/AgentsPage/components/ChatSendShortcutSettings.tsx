import { type FC, useId } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import { Switch } from "#/components/Switch/Switch";
import {
	DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
	MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
} from "../utils/agentChatSendShortcut";

export const ChatSendShortcutSettings: FC = () => {
	const queryClient = useQueryClient();
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));
	const descriptionId = useId();
	const shortcut =
		query.data?.agent_chat_send_shortcut ?? DEFAULT_AGENT_CHAT_SEND_SHORTCUT;
	const requiresModifierEnter = shortcut === MODIFIER_AGENT_CHAT_SEND_SHORTCUT;

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Keyboard shortcuts
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
						mutation.mutate({
							agent_chat_send_shortcut: checked
								? MODIFIER_AGENT_CHAT_SEND_SHORTCUT
								: DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
						})
					}
					aria-label="Require Cmd/Ctrl+Enter to send messages"
					aria-describedby={descriptionId}
					disabled={query.isLoading || !query.data || mutation.isPending}
				/>
			</div>
			{mutation.isError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save your keyboard shortcut preference.
				</p>
			)}
		</div>
	);
};
