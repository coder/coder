import { Spinner } from "components/Spinner/Spinner";
import { TerminalIcon } from "lucide-react";
import type { FC } from "react";

interface TerminalPanelProps {
	terminalHref: string | null;
	isExpanded: boolean;
}

export const TerminalPanel: FC<TerminalPanelProps> = ({
	terminalHref,
	isExpanded: _isExpanded,
}) => {
	if (!terminalHref) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="h-6 w-6" />
				<span className="text-sm">Waiting for workspace to connect...</span>
			</div>
		);
	}

	return (
		<iframe
			src={terminalHref}
			title="Terminal"
			className="h-full w-full border-0 bg-black"
			style={{ display: "block" }}
		/>
	);
};
