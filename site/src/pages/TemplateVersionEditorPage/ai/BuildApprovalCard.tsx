import { Button } from "components/Button/Button";
import { HammerIcon, LoaderIcon } from "lucide-react";
import type { FC } from "react";
import type { DisplayToolCall } from "./useTemplateAgent";

interface BuildApprovalCardProps {
	toolCall: DisplayToolCall;
	isPending: boolean;
	onApprove: () => void;
	onReject: () => void;
}

export const BuildApprovalCard: FC<BuildApprovalCardProps> = ({
	toolCall,
	isPending,
	onApprove,
	onReject,
}) => {
	const isRunning = toolCall.state === "pending" && !isPending;
	const result =
		typeof toolCall.result === "object" && toolCall.result !== null
			? (toolCall.result as Record<string, unknown>)
			: null;
	const resultError = typeof result?.error === "string" ? result.error : null;

	return (
		<div className="space-y-2 rounded-md border border-solid border-border p-3">
			<div className="flex items-center gap-2 text-xs font-medium text-content-primary">
				{isRunning ? (
					<LoaderIcon className="size-3.5 animate-spin" />
				) : (
					<HammerIcon className="size-3.5" />
				)}
				<span>
					{isPending
						? "Build template?"
						: isRunning
							? "Building…"
							: `Build ${result?.status ?? "complete"}`}
				</span>
			</div>

			{isPending && (
				<div className="flex gap-2">
					<Button size="sm" onClick={onApprove}>
						Approve
					</Button>
					<Button size="sm" variant="outline" onClick={onReject}>
						Reject
					</Button>
				</div>
			)}

			{resultError && (
				<p className="m-0 text-xs text-content-destructive">{resultError}</p>
			)}
		</div>
	);
};
