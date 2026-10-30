import { cn } from "cn";
import {
	CircleCheckIcon,
	CircleSlashIcon,
	CircleXIcon,
	type LucideIcon,
	MessageCircleReplyIcon,
	PlugZapIcon,
} from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { chatProjectTriggerDeliveries } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectTrigger,
	ChatProjectTriggerDelivery,
	ChatProjectTriggerDeliveryStatus,
	ChatProjectTriggerSource,
} from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { Loader } from "#/components/Loader/Loader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { DATE_FORMAT, timeFrom } from "#/utils/time";
import { buildAgentChatPath } from "../../utils/navigation";
import { SectionError } from "./ProjectSection";
import { triggerSources } from "./triggerCatalog";
import { formatInTimeZone } from "./triggerSchedule";

const deliveryStatuses: Record<
	ChatProjectTriggerDeliveryStatus,
	{
		readonly label: string;
		readonly className: string;
		readonly icon: LucideIcon;
	}
> = {
	started: {
		label: "Started a chat",
		className: "text-content-success",
		icon: CircleCheckIcon,
	},
	continued: {
		label: "Continued a chat",
		className: "text-content-success",
		icon: MessageCircleReplyIcon,
	},
	ignored: {
		label: "Ignored",
		className: "text-content-warning",
		icon: CircleSlashIcon,
	},
	rejected: {
		label: "Rejected",
		className: "text-content-destructive",
		icon: CircleXIcon,
	},
	// GitHub pings and Slack URL checks, sent while setting up the webhook.
	handshake: {
		label: "Connected",
		className: "text-content-secondary",
		icon: PlugZapIcon,
	},
};

type ChatLinkProps = {
	readonly chatId: string;
	readonly children: React.ReactNode;
};

const ChatLink: React.FC<ChatLinkProps> = ({ chatId, children }) => (
	<Link asChild size="sm" showExternalIcon={false}>
		<RouterLink to={buildAgentChatPath({ chatId })}>{children}</RouterLink>
	</Link>
);

/** The time relative times count from, and the time zone exact times show in. */
type DeliveryClockProps = {
	readonly now: Date;
	readonly timeZone: string;
};

type TriggerDeliveriesProps = DeliveryClockProps & {
	readonly project: ChatProject;
	readonly trigger: ChatProjectTrigger;
};

/**
 * One line under a trigger with what its last webhook call or run did,
 * such as "Ignored 2 minutes ago The trigger does not listen for push.",
 * and a button for the recent ones.
 */
export const TriggerDeliverySummary: React.FC<TriggerDeliveriesProps> = ({
	project,
	trigger,
	now,
	timeZone,
}) => {
	const delivery = trigger.last_delivery;
	if (!delivery) {
		return (
			<p className="m-0 text-xs text-content-secondary">
				{triggerSources[trigger.source].historyEmpty}
			</p>
		);
	}
	const status = deliveryStatuses[delivery.status];
	const StatusIcon = status.icon;
	return (
		<div className="flex min-w-0 items-start gap-1.5 text-xs leading-5">
			<StatusIcon
				aria-hidden
				className={cn("mt-1 size-icon-xs shrink-0", status.className)}
			/>
			<p className="m-0 min-w-0 flex-1 wrap-anywhere">
				{delivery.chat_id ? (
					<ChatLink chatId={delivery.chat_id}>{status.label}</ChatLink>
				) : (
					<span className={cn("font-medium", status.className)}>
						{status.label}
					</span>
				)}{" "}
				<time dateTime={delivery.created_at}>
					{timeFrom(delivery.created_at, now)}
				</time>
				{delivery.message && (
					<span className="text-content-secondary"> {delivery.message}</span>
				)}
			</p>
			<TriggerDeliveriesPopover
				project={project}
				trigger={trigger}
				now={now}
				timeZone={timeZone}
			/>
		</div>
	);
};

/** Loads the trigger's recent webhook calls or runs while it is open. */
const TriggerDeliveriesPopover: React.FC<TriggerDeliveriesProps> = ({
	project,
	trigger,
	now,
	timeZone,
}) => {
	const [open, setOpen] = useState(false);
	const { historyTitle } = triggerSources[trigger.source];
	const deliveriesQuery = useQuery({
		...chatProjectTriggerDeliveries(project, trigger.id),
		enabled: open,
	});
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Link asChild size="sm" showExternalIcon={false}>
					<button
						type="button"
						aria-label={`${historyTitle} for ${trigger.name}`}
						className="shrink-0 border-0 bg-transparent p-0"
					>
						{historyTitle}
					</button>
				</Link>
			</PopoverTrigger>
			<PopoverContent align="end" className="flex w-96 flex-col gap-2 p-3">
				<h3 className="m-0 text-sm font-medium text-content-primary">
					{historyTitle}
				</h3>
				<TriggerDeliveryList
					deliveries={deliveriesQuery.data}
					source={trigger.source}
					now={now}
					timeZone={timeZone}
					error={deliveriesQuery.error}
					onRetry={() => void deliveriesQuery.refetch()}
				/>
			</PopoverContent>
		</Popover>
	);
};

type TriggerDeliveryListProps = DeliveryClockProps & {
	/** `undefined` until the first load finishes. */
	readonly deliveries: readonly ChatProjectTriggerDelivery[] | undefined;
	readonly source: ChatProjectTriggerSource;
	readonly error: unknown;
	readonly onRetry: () => void;
};

const TriggerDeliveryList: React.FC<TriggerDeliveryListProps> = ({
	deliveries,
	source,
	now,
	timeZone,
	error,
	onRetry,
}) => {
	if (deliveries === undefined) {
		return error ? (
			<SectionError error={error} onRetry={onRetry} />
		) : (
			<Loader size="sm" label="Loading deliveries" className="p-4" />
		);
	}
	return (
		<>
			{/* A failed refresh keeps the list it already has. */}
			{Boolean(error) && <SectionError error={error} onRetry={onRetry} />}
			{deliveries.length === 0 ? (
				<p className="m-0 text-xs text-content-secondary">
					{triggerSources[source].historyEmpty}
				</p>
			) : (
				<ul className="m-0 flex max-h-80 list-none flex-col divide-y divide-border overflow-y-auto p-0">
					{deliveries.map((delivery) => (
						<TriggerDeliveryItem
							key={delivery.id}
							delivery={delivery}
							source={source}
							now={now}
							timeZone={timeZone}
						/>
					))}
				</ul>
			)}
		</>
	);
};

type TriggerDeliveryItemProps = DeliveryClockProps & {
	readonly delivery: ChatProjectTriggerDelivery;
	readonly source: ChatProjectTriggerSource;
};

const TriggerDeliveryItem: React.FC<TriggerDeliveryItemProps> = ({
	delivery,
	source,
	now,
	timeZone,
}) => {
	const status = deliveryStatuses[delivery.status];
	const StatusIcon = status.icon;
	const eventLabel = delivery.event
		? triggerSources[source].deliveryEventLabels?.[delivery.event]
		: undefined;
	return (
		<li className="flex gap-2 py-2 text-xs first:pt-0 last:pb-0">
			<StatusIcon
				aria-hidden
				className={cn("mt-px size-icon-xs shrink-0", status.className)}
			/>
			<div className="flex min-w-0 flex-1 flex-col gap-0.5">
				<div className="flex items-baseline gap-2">
					<span className={cn("font-medium", status.className)}>
						{status.label}
					</span>
					{eventLabel ? (
						<span className="min-w-0 truncate text-content-secondary">
							{eventLabel}
						</span>
					) : (
						delivery.event && (
							<code className="min-w-0 truncate font-mono text-content-secondary">
								{delivery.event}
							</code>
						)
					)}
					<time
						dateTime={delivery.created_at}
						className="ml-auto shrink-0 text-content-secondary"
					>
						{timeFrom(delivery.created_at, now)}
					</time>
				</div>
				{delivery.message && (
					<p className="m-0 wrap-anywhere text-content-secondary">
						{delivery.message}
					</p>
				)}
				<div className="flex items-center gap-2 text-content-secondary">
					<span>
						{formatInTimeZone(
							delivery.created_at,
							timeZone,
							DATE_FORMAT.FULL_DATETIME,
						)}
					</span>
					{delivery.chat_id && (
						<ChatLink chatId={delivery.chat_id}>Open chat</ChatLink>
					)}
				</div>
			</div>
		</li>
	);
};
