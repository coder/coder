import type React from "react";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";

/** Markdown in a bordered region that scrolls past a fixed height. */
export const ScrollableMarkdown: React.FC<{
	ariaLabel: string;
	children: string;
}> = ({ ariaLabel, children }) => (
	<ToolCall.Scrollable
		className="mt-1.5 rounded-md border border-solid border-border"
		viewportClassName="max-h-64"
		ariaLabel={ariaLabel}
	>
		<div className="px-3 py-2">
			<Response>{children}</Response>
		</div>
	</ToolCall.Scrollable>
);
