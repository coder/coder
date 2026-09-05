import type { FC } from "react";
import { Switch } from "#/components/Switch/Switch";
import { useStorage } from "#/hooks/useStorage";
import { chatFullWidthStorage } from "../storage";

export const ChatFullWidthSettings: FC = () => {
	const [enabled, setEnabled] = useStorage(chatFullWidthStorage);

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Chat layout
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 flex-1 text-xs text-content-secondary">
					Use full-width layout for agent chat messages, removing the default
					max-width constraint.
				</p>
				<Switch
					checked={enabled}
					onCheckedChange={(checked) => setEnabled(Boolean(checked))}
					aria-label="Full-width chat"
				/>
			</div>
		</div>
	);
};
