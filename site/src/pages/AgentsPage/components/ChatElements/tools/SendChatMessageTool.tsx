import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useLocation } from "react-router";
import { safeBuildAgentChatPath } from "../../../utils/navigation";
import { senderChatRelationLabels } from "../../ChatConversation/SenderChatHeader";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

type SendChatMessageDelivery = "started" | "queued" | "interrupting";

interface SendChatMessageToolProps {
	/** Resolved target chat id, or the literal "parent" from the arguments. */
	readonly targetChatId?: string;
	readonly targetTitle?: string;
	readonly relation?: string;
	readonly message?: string;
	readonly requestedDelivery?: string;
	readonly delivery?: SendChatMessageDelivery | string;
	readonly downgradedFrom?: string;
	readonly previousStatus?: string;
	readonly targetStatus?: string;
	readonly relayHop?: number;
	readonly status: ToolStatus;
	readonly isError: boolean;
	readonly errorMessage?: string;
}

const DELIVERY_LABELS: Record<SendChatMessageDelivery, string> = {
	started: "started a new turn",
	queued: "queued until the current turn ends",
	interrupting: "interrupting the current turn",
};

const isKnownDelivery = (
	delivery: string,
): delivery is SendChatMessageDelivery =>
	Object.hasOwn(DELIVERY_LABELS, delivery);

const deliveryLabel = (delivery: string | undefined): string | undefined => {
	if (!delivery) {
		return undefined;
	}
	return isKnownDelivery(delivery) ? DELIVERY_LABELS[delivery] : delivery;
};

const isKnownRelation = (
	relation: string,
): relation is keyof typeof senderChatRelationLabels =>
	Object.hasOwn(senderChatRelationLabels, relation);

/** Sentence-case relation label for the "Target" row. */
const relationLabelFor = (relation: string | undefined): string | undefined => {
	if (!relation || !isKnownRelation(relation)) {
		return undefined;
	}
	const label = senderChatRelationLabels[relation];
	return label.charAt(0).toUpperCase() + label.slice(1);
};

/**
 * Picks the chat id shown and linked for a `send_chat_message` call. The
 * result's id wins. For results that did not fail the model-supplied
 * argument is used as is; for failed results only the literal "parent"
 * is kept, which labels the target and is never linked.
 */
export const resolveSendChatMessageTargetId = (
	resultChatId: string | undefined,
	argsChatId: string | undefined,
	isError: boolean,
): string | undefined => {
	if (resultChatId) {
		return resultChatId;
	}
	if (isError) {
		return argsChatId === "parent" ? argsChatId : undefined;
	}
	return argsChatId || undefined;
};

const targetLabel = (
	targetTitle: string | undefined,
	targetChatId: string | undefined,
) => {
	if (targetTitle) {
		return `Message to ${targetTitle}`;
	}
	if (targetChatId === "parent") {
		return "Message to parent chat";
	}
	return "Message to chat";
};

/**
 * Renders a `send_chat_message` tool call: the target chat, how the
 * message was delivered, and the message text under the disclosure.
 */
export const SendChatMessageTool: FC<SendChatMessageToolProps> = ({
	targetChatId,
	targetTitle,
	relation,
	message,
	requestedDelivery,
	delivery,
	downgradedFrom,
	previousStatus,
	targetStatus,
	relayHop,
	status,
	isError,
	errorMessage,
}) => {
	const location = useLocation();
	const isRunning = status === "running";
	const targetPath =
		targetChatId && targetChatId !== "parent"
			? safeBuildAgentChatPath({ chatId: targetChatId })
			: null;
	const label = isRunning
		? "Sending message"
		: targetLabel(targetTitle, targetChatId);
	const relationLabel = relationLabelFor(relation);
	const rows: { key: string; label: string; value: string }[] = [];
	if (relationLabel) {
		rows.push({ key: "relation", label: "Target", value: relationLabel });
	}
	const effectiveDelivery = deliveryLabel(delivery);
	if (effectiveDelivery) {
		rows.push({ key: "delivery", label: "Delivery", value: effectiveDelivery });
	} else if (requestedDelivery) {
		rows.push({
			key: "requested",
			label: "Requested",
			value: requestedDelivery,
		});
	}
	if (downgradedFrom) {
		rows.push({
			key: "downgraded",
			label: "Downgraded from",
			value: `${downgradedFrom} (the target was waiting for approval)`,
		});
	}
	if (previousStatus || targetStatus) {
		rows.push({
			key: "status",
			label: "Target status",
			value:
				previousStatus && targetStatus && previousStatus !== targetStatus
					? `${previousStatus} to ${targetStatus}`
					: (targetStatus ?? previousStatus ?? ""),
		});
	}
	if (relayHop !== undefined && relayHop > 1) {
		rows.push({ key: "hop", label: "Relay hop", value: String(relayHop) });
	}
	const hasContent = rows.length > 0 || Boolean(message) || targetPath !== null;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to send the message"}
			hasContent={hasContent}
		>
			<ToolCall.Header iconName="send_chat_message" label={label} />
			<ToolCall.Content>
				<div className="mt-1.5 flex flex-col gap-1.5 text-[13px] text-content-secondary">
					{rows.length > 0 && (
						<dl className="m-0 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
							{rows.map((row) => (
								<div key={row.key} className="contents">
									<dt className="text-content-secondary/70">{row.label}</dt>
									<dd className="m-0 text-content-primary">{row.value}</dd>
								</div>
							))}
						</dl>
					)}
					{message && (
						<blockquote className="m-0 whitespace-pre-wrap border-0 border-l-2 border-solid border-border-default pl-2 text-content-primary">
							{message}
						</blockquote>
					)}
					{targetPath && (
						<Link
							to={{ pathname: targetPath, search: location.search }}
							className="flex w-fit items-center gap-1.5 text-[13px] text-content-secondary opacity-50 transition-opacity hover:opacity-100"
						>
							Open {targetTitle ?? "target chat"}
							<ExternalLinkIcon className="size-3 shrink-0" />
						</Link>
					)}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
