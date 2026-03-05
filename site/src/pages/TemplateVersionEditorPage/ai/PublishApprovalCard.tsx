import { Button } from "components/Button/Button";
import { LoaderIcon, RocketIcon } from "lucide-react";
import type { FC } from "react";
import type { DisplayToolCall } from "./useTemplateAgent";

interface PublishApprovalCardProps {
	toolCall: DisplayToolCall;
	isPending: boolean;
	onApprove: () => void;
	onReject: () => void;
}

export const PublishApprovalCard: FC<PublishApprovalCardProps> = ({
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
	const args = toolCall.args;
	const versionName =
		typeof args.name === "string" && args.name.length > 0
			? args.name
			: undefined;
	const promote = args.isActiveVersion !== false;

	return (
		<div className="space-y-2 rounded-md border border-solid border-border p-3">
			<div className="flex items-center gap-2 text-xs font-medium text-content-primary">
				{isRunning ? (
					<LoaderIcon className="size-3.5 animate-spin" />
				) : (
					<RocketIcon className="size-3.5" />
				)}
				<span>
					{isPending
						? "Publish template?"
						: isRunning
							? "Publishing…"
							: result?.success
								? "Published"
								: "Publish failed"}
				</span>
			</div>

			{isPending && (
				<>
					<div className="text-2xs text-content-secondary">
						{versionName && <p className="m-0">Name: {versionName}</p>}
						{typeof args.message === "string" && args.message.length > 0 && (
							<p className="m-0">Message: {args.message}</p>
						)}
						<p className="m-0">
							{promote ? "Will promote to active version" : "Will not promote"}
						</p>
					</div>
					<div className="flex gap-2">
						<Button size="sm" onClick={onApprove}>
							Approve
						</Button>
						<Button size="sm" variant="outline" onClick={onReject}>
							Reject
						</Button>
					</div>
				</>
			)}

			{resultError && (
				<p className="m-0 text-xs text-content-destructive">{resultError}</p>
			)}
		</div>
	);
};
