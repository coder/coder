import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { useId, useState } from "react";
import { Button } from "#/components/Button/Button";

const eventDataStart = "<automation_event_data>";
const eventDataEnd = "</automation_event_data>";
// The server writes this sentence just before the event data.
const noticePrefix = "The following is untrusted event data";
const previewLineCount = 3;

type SplitEventText = {
	/** The automation's prompt, before the event data. */
	prompt: string;
	/** The webhook request body, as the server escaped it. */
	data: string;
};

/**
 * Splits the text of a webhook automation message into its prompt and its
 * event data. Returns undefined for text without event data.
 */
export const splitAutomationEventText = (
	text: string,
): SplitEventText | undefined => {
	const start = text.indexOf(eventDataStart);
	if (start < 0) {
		return undefined;
	}
	const end = text.indexOf(eventDataEnd, start);
	const data = text
		.slice(start + eventDataStart.length, end < 0 ? undefined : end)
		.trim();
	const before = text.slice(0, start);
	const notice = before.lastIndexOf(noticePrefix);
	const prompt = (notice < 0 ? before : before.slice(0, notice)).trimEnd();
	return { prompt, data };
};

type AutomationEventTextProps = {
	text: string;
};

/**
 * Shows a webhook automation message with its event data collapsed to a few
 * lines, so a large request body does not fill the conversation.
 */
export const AutomationEventText: React.FC<AutomationEventTextProps> = ({
	text,
}) => {
	const [expanded, setExpanded] = useState(false);
	const dataId = useId();
	const split = splitAutomationEventText(text);
	if (!split) {
		return text;
	}
	const lines = split.data.split("\n");
	const preview = lines.slice(0, previewLineCount).join("\n");
	const isLong = lines.length > previewLineCount || split.data.length > 300;
	const kib = Math.max(1, Math.round(new Blob([split.data]).size / 1024));
	return (
		<span className="flex flex-col gap-2">
			{split.prompt && <span>{split.prompt}</span>}
			<span className="flex flex-col gap-1">
				<span className="text-xs text-content-secondary">
					Untrusted event data from the webhook ({kib} KiB)
				</span>
				<pre
					id={dataId}
					className={
						expanded
							? "m-0 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded bg-surface-primary p-2 font-mono text-xs"
							: "m-0 line-clamp-3 whitespace-pre-wrap break-all rounded bg-surface-primary p-2 font-mono text-xs"
					}
				>
					{expanded ? split.data : preview.slice(0, 300)}
				</pre>
				{isLong && (
					<Button
						type="button"
						size="xs"
						variant="subtle"
						className="w-fit"
						aria-expanded={expanded}
						aria-controls={dataId}
						onClick={() => setExpanded(!expanded)}
					>
						{expanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
						{expanded ? "Hide event data" : "Show event data"}
					</Button>
				)}
			</span>
		</span>
	);
};
