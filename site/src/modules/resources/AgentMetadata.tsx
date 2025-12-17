import Skeleton from "@mui/material/Skeleton";
import { watchAgentMetadataPolling } from "api/api";
import type {
	ServerSentEvent,
	WorkspaceAgent,
	WorkspaceAgentMetadata,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import dayjs from "dayjs";
import {
	type FC,
	type HTMLAttributes,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import type { OneWayWebSocket } from "utils/OneWayWebSocket";

type ItemStatus = "stale" | "valid" | "loading";

interface AgentMetadataViewProps {
	metadata: WorkspaceAgentMetadata[];
}

export const AgentMetadataView: FC<AgentMetadataViewProps> = ({ metadata }) => {
	if (metadata.length === 0) {
		return null;
	}
	return (
		<section className="flex items-baseline flex-wrap gap-8 gap-y-4">
			{metadata.map((m) => (
				<MetadataItem key={m.description.key} item={m} />
			))}
		</section>
	);
};

interface AgentMetadataProps {
	agent: WorkspaceAgent;
	initialMetadata?: WorkspaceAgentMetadata[];
}

const maxSocketErrorRetryCount = 3;

export const AgentMetadata: FC<AgentMetadataProps> = ({
	agent,
	initialMetadata,
}) => {
	const [activeMetadata, setActiveMetadata] = useState(initialMetadata);

	useEffect(() => {
		// This is an unfortunate pitfall with this component's testing setup,
		// but even though we use the value of initialMetadata as the initial
		// value of the activeMetadata, we cannot put activeMetadata itself into
		// the dependency array. If we did, we would destroy and rebuild each
		// connection every single time a new message comes in from the socket,
		// because the socket has to be wired up to the state setter
		if (initialMetadata !== undefined) {
			return;
		}

		let timeoutId: number | undefined;
		let activeSocket: OneWayWebSocket<ServerSentEvent> | null = null;
		let retries = 0;

		const createNewConnection = () => {
			const socket = watchAgentMetadataPolling(agent.id);
			activeSocket = socket;

			socket.addEventListener("error", () => {
				setActiveMetadata(undefined);
				window.clearTimeout(timeoutId);

				// The error event is supposed to fire when an error happens
				// with the connection itself, which implies that the connection
				// would auto-close. Couldn't find a definitive answer on MDN,
				// though, so closing it manually just to be safe
				socket.close();
				activeSocket = null;

				retries++;
				if (retries >= maxSocketErrorRetryCount) {
					displayError(
						"Unexpected disconnect while watching Metadata changes. Please try refreshing the page.",
					);
					return;
				}

				displayError(
					"Unexpected disconnect while watching Metadata changes. Creating new connection...",
				);
				timeoutId = window.setTimeout(() => {
					createNewConnection();
				}, 3_000);
			});

			socket.addEventListener("message", (e) => {
				if (e.parseError) {
					displayError(
						"Unable to process newest response from server. Please try refreshing the page.",
					);
					return;
				}

				const msg = e.parsedMessage;
				if (msg.type === "data") {
					setActiveMetadata(msg.data as WorkspaceAgentMetadata[]);
				}
			});
		};

		createNewConnection();
		return () => {
			window.clearTimeout(timeoutId);
			activeSocket?.close();
		};
	}, [agent.id, initialMetadata]);

	if (activeMetadata === undefined) {
		return (
			<section className="flex items-baseline flex-wrap gap-8 gap-y-4">
				<AgentMetadataSkeleton />
			</section>
		);
	}

	return <AgentMetadataView metadata={activeMetadata} />;
};

const AgentMetadataSkeleton: FC = () => {
	return (
		<Stack alignItems="baseline" direction="row" spacing={6}>
			<div className="leading-relaxed flex flex-col overflow-visible flex-shrink-0">
				<Skeleton width={40} height={12} variant="text" />
				<Skeleton width={65} height={14} variant="text" />
			</div>

			<div className="leading-relaxed flex flex-col overflow-visible flex-shrink-0">
				<Skeleton width={40} height={12} variant="text" />
				<Skeleton width={65} height={14} variant="text" />
			</div>

			<div className="leading-relaxed flex flex-col overflow-visible flex-shrink-0">
				<Skeleton width={40} height={12} variant="text" />
				<Skeleton width={65} height={14} variant="text" />
			</div>
		</Stack>
	);
};

interface MetadataItemProps {
	item: WorkspaceAgentMetadata;
}

const MetadataItem: FC<MetadataItemProps> = ({ item }) => {
	const staleThreshold = Math.max(
		item.description.interval + item.description.timeout * 2,
		// In case there is intense backpressure, we give a little bit of slack.
		5,
	);

	const status: ItemStatus = (() => {
		const year = dayjs(item.result.collected_at).year();
		if (year <= 1970 || Number.isNaN(year)) {
			return "loading";
		}
		// There is a special circumstance for metadata with `interval: 0`. It is
		// expected that they run once and never again, so never display them as
		// stale.
		if (item.result.age > staleThreshold && item.description.interval > 0) {
			return "stale";
		}
		return "valid";
	})();

	// Stale data is as good as no data. Plus, we want to build confidence in our
	// users that what's shown is real. If times aren't correctly synced this
	// could be buggy. But, how common is that anyways?
	const value =
		status === "loading" ? (
			<Skeleton width={65} height={12} variant="text" className="mt-[6px]" />
		) : status === "stale" ? (
			<Tooltip>
				<TooltipTrigger asChild>
					<StaticWidth className="text-ellipsis overflow-hidden whitespace-nowrap max-w-64 text-sm text-content-disabled cursor-pointer">
						{item.result.value}
					</StaticWidth>
				</TooltipTrigger>
				<TooltipContent side="bottom">
					This data is stale and no longer up to date
				</TooltipContent>
			</Tooltip>
		) : (
			<StaticWidth
				className={cn(
					"text-ellipsis overflow-hidden whitespace-nowrap max-w-64 text-sm text-content-success",
					item.result.error.length > 0 && "text-content-destructive",
				)}
			>
				{item.result.value}
			</StaticWidth>
		);

	return (
		<div className="leading-relaxed flex flex-col overflow-visible flex-shrink-0">
			<div className="text-content-secondary text-ellipsis overflow-hidden whitespace-nowrap text-[13px]">
				{item.description.display_name}
			</div>
			<div>{value}</div>
		</div>
	);
};

const StaticWidth: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	const ref = useRef<HTMLDivElement>(null);

	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useLayoutEffect(() => {
		// Ignore this in storybook
		if (!ref.current || process.env.STORYBOOK === "true") {
			return;
		}

		const currentWidth = ref.current.getBoundingClientRect().width;
		ref.current.style.width = "auto";
		const autoWidth = ref.current.getBoundingClientRect().width;
		ref.current.style.width =
			autoWidth > currentWidth ? `${autoWidth}px` : `${currentWidth}px`;
	}, [children]);

	return (
		<div ref={ref} {...attrs}>
			{children}
		</div>
	);
};
