import { cn } from "cn";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { RetryButton } from "./RetryButton";

type AttachmentsErrorAlertProps = {
	isRetrying: boolean;
	onRetry: () => void;
	className?: string;
};

/**
 * One line, because the attachments request error is too tall for the
 * overview card and the attachments list. The request message wraps, and
 * ErrorAlert also renders the error detail and stack trace.
 */
export const AttachmentsErrorAlert: React.FC<AttachmentsErrorAlertProps> = ({
	isRetrying,
	onRetry,
	className,
}) => (
	<ErrorAlert
		error="Failed to get attachments"
		showDebugDetail={false}
		className={cn(
			"p-2",
			// Alert stacks its message and actions. These pull them onto one row.
			"[&>div>div]:items-center",
			"[&>div>div>div]:flex-row [&>div>div>div]:items-center [&>div>div>div]:gap-2",
			"[&_h2]:text-xs [&_svg]:mt-0 [&_svg]:size-3.5",
			className,
		)}
		actions={
			<RetryButton
				variant="outline"
				size="xs"
				isRetrying={isRetrying}
				onRetry={onRetry}
			/>
		}
	/>
);
