import { type FC, useId } from "react";
import { Switch } from "#/components/Switch/Switch";
import { useVimNavigation } from "../hooks/useVimNavigation";

export const ChatVimNavigationSettings: FC = () => {
	const [enabled, setEnabled] = useVimNavigation();
	const descriptionId = useId();

	return (
		<div className="flex items-center justify-between gap-4">
			<p
				id={descriptionId}
				className="m-0 flex-1 text-xs text-content-secondary"
			>
				Vim-style navigation. Cmd/Ctrl+J and Cmd/Ctrl+K select the next and
				previous chat, Cmd/Ctrl+Shift+J and Cmd/Ctrl+Shift+K jump to the last
				and first chat, Cmd/Ctrl+Shift+O starts a new chat, Cmd/Ctrl+Shift+E
				renames the current chat, and Escape on a sidebar chat returns focus to
				the message input. Search moves from Cmd/Ctrl+K to Cmd/Ctrl+/. These
				override browser shortcuts on the same keys.
			</p>
			<Switch
				checked={enabled}
				onCheckedChange={(checked) => setEnabled(Boolean(checked))}
				aria-label="Vim-style chat navigation"
				aria-describedby={descriptionId}
			/>
		</div>
	);
};
