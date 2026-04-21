import type { FC } from "react";
import { Switch } from "#/components/Switch/Switch";
import { useChatFullWidth } from "../hooks/useChatFullWidth";

export const ChatFullWidthSettings: FC = () => {
	const [enabled, setEnabled] = useChatFullWidth();

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Chat Layout
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
