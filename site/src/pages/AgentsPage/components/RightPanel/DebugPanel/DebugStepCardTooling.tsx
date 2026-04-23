import { WrenchIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { cn } from "#/utils/cn";
import { CopyableCodeBlock, RoleBadge } from "./DebugPanelPrimitives";
import {
	clampContent,
	exceedsClampThreshold,
	MESSAGE_CONTENT_CLAMP_CHARS,
	type MessagePart,
} from "./debugPanelUtils";

interface MessageRowProps {
	msg: MessagePart;
	clamp: boolean;
}

interface ToolPayloadDisclosureProps {
	label: string;
	code?: string;
	copyLabel: string;
}

export const ToolPayloadDisclosure: FC<ToolPayloadDisclosureProps> = ({
	label,
	code,
	copyLabel,
}) => {
	if (!code) {
		return null;
	}

	return (
		<div className="mt-2 space-y-1">
			<p className="text-2xs font-medium uppercase tracking-wide text-content-tertiary">
				{label}
			</p>
			<CopyableCodeBlock code={code} label={copyLabel} className="max-h-56" />
		</div>
	);
};

export const ToolBadge: FC<{ label: string }> = ({ label }) => {
	return (
		<Badge size="sm" variant="purple" className="max-w-full">
			<WrenchIcon className="size-3 shrink-0" />
			<span className="truncate">{label}</span>
		</Badge>
	);
};

interface ToolEventCardProps {
	badgeLabel: string;
	toolCallId?: string;
	payloadLabel?: string;
	payload?: string;
	copyLabel?: string;
}

export const ToolEventCard: FC<ToolEventCardProps> = ({
	badgeLabel,
	toolCallId,
	payloadLabel,
	payload,
	copyLabel,
}) => {
	return (
		<div className="rounded-md border border-solid border-border-default/40 bg-surface-secondary/10 p-2.5">
			<div className="flex min-w-0 flex-wrap items-center gap-2">
				<ToolBadge label={badgeLabel} />
				{toolCallId ? (
					<span className="min-w-0 truncate font-mono text-2xs text-content-tertiary">
						{toolCallId}
					</span>
				) : null}
			</div>
			{payloadLabel && payload && copyLabel ? (
				<ToolPayloadDisclosure
					label={payloadLabel}
					code={payload}
					copyLabel={copyLabel}
				/>
			) : null}
		</div>
	);
};

const TranscriptToolRow: FC<{ msg: MessagePart }> = ({ msg }) => {
	const isToolCall = msg.kind === "tool-call";
	const badgeLabel = msg.toolName ?? (isToolCall ? "Tool call" : "Tool result");
	const payloadLabel = isToolCall ? "Arguments" : "Result";
	const payload = isToolCall ? msg.arguments : msg.result;

	return (
		<div className="space-y-1.5">
			<div className="flex items-center gap-2">
				<RoleBadge role={msg.role} />
			</div>
			<ToolEventCard
				badgeLabel={badgeLabel}
				toolCallId={msg.toolCallId}
				payloadLabel={payloadLabel}
				payload={payload}
				copyLabel={`Copy ${badgeLabel} ${payloadLabel}`}
			/>
		</div>
	);
};

const TranscriptTextRow: FC<MessageRowProps> = ({ msg, clamp }) => {
	const [expanded, setExpanded] = useState(false);
	// Use the same code-point count as clampContent so the "see more"
	// control never appears when the message is short enough that
	// clampContent would return it unchanged.
	const needsClamp =
		clamp && exceedsClampThreshold(msg.content, MESSAGE_CONTENT_CLAMP_CHARS);
	const showClamped = needsClamp && !expanded;
	const displayContent = showClamped
		? clampContent(msg.content, MESSAGE_CONTENT_CLAMP_CHARS)
		: msg.content;

	return (
		<div className="space-y-0.5">
			<div className="flex items-center gap-2">
				<RoleBadge role={msg.role} />
				{msg.toolName ? (
					<span className="min-w-0 truncate font-mono text-2xs text-content-tertiary">
						{msg.toolName}
					</span>
				) : null}
				{msg.toolCallId && !msg.toolName ? (
					<span className="min-w-0 truncate font-mono text-2xs text-content-tertiary">
						{msg.toolCallId}
					</span>
				) : null}
			</div>
			{displayContent ? (
				<>
					<p
						className={cn(
							"whitespace-pre-wrap text-xs leading-5 text-content-primary",
							showClamped && "line-clamp-3",
						)}
					>
						{displayContent}
					</p>
					{needsClamp ? (
						<button
							type="button"
							onClick={() => setExpanded((prev) => !prev)}
							className="border-0 bg-transparent p-0 text-2xs font-medium text-content-link transition-colors hover:underline"
							aria-label={`See ${expanded ? "less" : "more"} of ${msg.role} message`}
						>
							{expanded ? "see less" : "see more"}
						</button>
					) : null}
				</>
			) : null}
		</div>
	);
};

export const MessageRow: FC<MessageRowProps> = ({ msg, clamp }) => {
	if (msg.kind === "tool-call" || msg.kind === "tool-result") {
		return <TranscriptToolRow msg={msg} />;
	}

	return <TranscriptTextRow msg={msg} clamp={clamp} />;
};
