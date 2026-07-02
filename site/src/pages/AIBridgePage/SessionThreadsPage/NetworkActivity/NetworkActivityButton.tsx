import {
	CheckIcon,
	ChevronRightIcon,
	NetworkIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";
import { formatDateTime } from "#/utils/time";
import {
	computeNetworkCounts,
	type NetworkActivity,
	type NetworkEvent,
	type NetworkFailureEvent,
	type NetworkRequestEvent,
} from "./types";

interface NetworkActivityButtonProps {
	networkActivity: NetworkActivity;
}

/**
 * Button + Popover combo. Triggered by the `Network activity (N)` button at
 * the top of the session timeline. The popover anchors to the trigger and
 * Radix auto-flips it above/below when there is not enough room. Internal
 * scrolling keeps the dialog from being cut off in small viewports.
 */
export const NetworkActivityButton: FC<NetworkActivityButtonProps> = ({
	networkActivity,
}) => {
	const counts = computeNetworkCounts(networkActivity);

	if (counts.total === 0) {
		return null;
	}

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button variant="outline" size="sm">
					<NetworkIcon />
					<span className="font-normal">Network activity ({counts.total})</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="start"
				side="bottom"
				sideOffset={8}
				className="flex flex-col overflow-hidden p-0 w-[min(36rem,calc(100vw-2rem))]"
			>
				<NetworkActivityDialog
					events={networkActivity.events}
					counts={counts}
				/>
			</PopoverContent>
		</Popover>
	);
};

interface NetworkActivityDialogProps {
	events: readonly NetworkEvent[];
	counts: ReturnType<typeof computeNetworkCounts>;
}

const NetworkActivityDialog: FC<NetworkActivityDialogProps> = ({
	events,
	counts,
}) => (
	<>
		<header className="border-0 border-b border-solid px-4 py-3 flex flex-col gap-1.5 shrink-0">
			<h2 className="m-0 text-sm font-semibold text-content-primary">
				Network activity ({counts.total})
			</h2>
			<dl className="m-0 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary">
				<SummaryCount
					tone="success"
					value={counts.allowed}
					label={counts.allowed === 1 ? "allowed" : "allowed"}
				/>
				<SummaryCount
					tone="warning"
					value={counts.warnings}
					label={counts.warnings === 1 ? "warning" : "warnings"}
				/>
				<SummaryCount
					tone="error"
					value={counts.errors}
					label={counts.errors === 1 ? "error" : "errors"}
				/>
			</dl>
		</header>
		<ul className="m-0 p-0 list-none overflow-y-auto flex-1 divide-y divide-solid divide-border-default">
			{events.map((event) => (
				<NetworkEventRow key={event.id} event={event} />
			))}
		</ul>
	</>
);

interface SummaryCountProps {
	value: number;
	label: string;
	tone: "success" | "warning" | "error";
}

const SummaryCount: FC<SummaryCountProps> = ({ value, label, tone }) => (
	<div className="inline-flex items-center gap-1.5">
		<ToneIcon tone={tone} />
		<span className="text-content-primary">
			{value} {label}
		</span>
	</div>
);

const ToneIcon: FC<{ tone: "success" | "warning" | "error" }> = ({ tone }) => {
	if (tone === "success") {
		return (
			<CheckIcon className="size-icon-xs p-0.5 text-content-success shrink-0" />
		);
	}
	if (tone === "warning") {
		return (
			<TriangleAlertIcon className="size-icon-xs p-0.5 text-content-warning shrink-0" />
		);
	}
	return (
		<TriangleAlertIcon className="size-icon-xs p-0.5 text-content-destructive shrink-0" />
	);
};

interface NetworkEventRowProps {
	event: NetworkEvent;
}

const NetworkEventRow: FC<NetworkEventRowProps> = ({ event }) => {
	const [isOpen, setIsOpen] = useState(false);
	const toggle = () => setIsOpen((v) => !v);

	return (
		<li className="m-0">
			<button
				type="button"
				onClick={toggle}
				aria-expanded={isOpen}
				className={cn(
					"w-full bg-transparent border-0 p-0 m-0 text-left cursor-pointer",
					"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
				)}
			>
				<div className="flex items-center gap-2 px-3 py-2 hover:bg-surface-secondary min-w-0">
					<ChevronRightIcon
						className={cn(
							"size-3.5 shrink-0 text-content-secondary transition-transform",
							isOpen && "rotate-90",
						)}
					/>
					{event.kind === "request" ? (
						<RequestHead event={event} />
					) : (
						<FailureHead event={event} />
					)}
				</div>
			</button>
			{isOpen && (
				<div className="px-3 pb-3 pl-9">
					{event.kind === "request" ? (
						<RequestDetails event={event} />
					) : (
						<FailureDetails event={event} />
					)}
				</div>
			)}
		</li>
	);
};

const RequestHead: FC<{ event: NetworkRequestEvent }> = ({ event }) => (
	<div className="flex items-center gap-2 min-w-0 flex-1">
		<Badge size="xs" className="font-mono shrink-0 min-w-[3rem] justify-center">
			{event.method}
		</Badge>
		<StatusBadge status={event.status} />
		<span
			className="text-xs font-mono text-content-primary truncate min-w-0 flex-1"
			title={event.url}
		>
			{event.url}
		</span>
	</div>
);

const FailureHead: FC<{ event: NetworkFailureEvent }> = ({ event }) => (
	<div className="flex items-center gap-2 min-w-0 flex-1">
		<Badge size="xs" variant="destructive" className="shrink-0 gap-1">
			<TriangleAlertIcon className="size-3" />
			{event.label}
		</Badge>
		<span
			className="text-xs font-mono text-content-primary truncate min-w-0 flex-1"
			title={event.detail}
		>
			{event.detail}
		</span>
	</div>
);

const StatusBadge: FC<{ status: NetworkRequestEvent["status"] }> = ({
	status,
}) => {
	if (status === "allowed") {
		return (
			<Badge size="xs" className="shrink-0 gap-1">
				<CheckIcon className="size-3 text-content-success" />
				Allowed
			</Badge>
		);
	}
	return (
		<Badge size="xs" variant="warning" className="shrink-0 gap-1">
			<TriangleAlertIcon className="size-3" />
			Blocked
		</Badge>
	);
};

const RequestDetails: FC<{ event: NetworkRequestEvent }> = ({ event }) => (
	<DetailGrid>
		<DetailRow label="Start time" value={formatDateTime(event.timestamp)} />
		{event.policy && <DetailRow label="Policy" value={event.policy} />}
		{event.error && <DetailRow label="Error" value={event.error} />}
		{event.policyConfigurationHref && (
			<PolicyConfigurationLink href={event.policyConfigurationHref} />
		)}
	</DetailGrid>
);

const FailureDetails: FC<{ event: NetworkFailureEvent }> = ({ event }) => (
	<DetailGrid>
		{event.description && (
			<p className="m-0 text-sm text-content-primary leading-snug">
				{event.description}
			</p>
		)}
		<DetailRow label="Timestamp" value={formatDateTime(event.timestamp)} />
		{event.policy && <DetailRow label="Policy" value={event.policy} />}
		{event.error && <DetailRow label="Error" value={event.error} />}
		{event.policyConfigurationHref && (
			<PolicyConfigurationLink href={event.policyConfigurationHref} />
		)}
	</DetailGrid>
);

const DetailGrid: FC<{ children: React.ReactNode }> = ({ children }) => (
	<div className="flex flex-col gap-1.5 text-sm">{children}</div>
);

const DetailRow: FC<{ label: string; value: string }> = ({ label, value }) => (
	<div className="flex items-baseline gap-3 min-w-0">
		<span className="text-content-secondary font-normal shrink-0">
			{label}:
		</span>
		<span
			className="text-content-primary font-mono text-xs truncate min-w-0 flex-1"
			title={value}
		>
			{value}
		</span>
	</div>
);

const PolicyConfigurationLink: FC<{ href: string }> = ({ href }) => (
	<Link href={href} size="sm" target="_blank" rel="noreferrer">
		View policy configuration
	</Link>
);
