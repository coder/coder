import { type FC, useEffect } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "react-query";
import { watchWorkspaceAISandboxActivity } from "#/api/api";
import {
	infiniteWorkspaceAISandboxNetworkEvents,
	workspaceAISandboxActivityKey,
	workspaceAISandboxSessions,
	workspaceAISandboxSessionsKey,
} from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import { formatDate } from "#/utils/time";

const enforcementVariant = {
	forced: "default",
	advisory: "warning",
	none: "destructive",
} as const;

const enforcementHelp = {
	forced: "All egress is routed through the platform proxy.",
	advisory: "Egress is routed cooperatively and can be bypassed.",
	none: "No egress routing is claimed for this sandbox.",
} as const;

interface WorkspaceAIEgressActivityProps {
	workspaceId: string;
}

// Shows the retained egress audit trail for AI-confined execution. The
// attestation badge reports what the template declared: the platform records
// it but cannot verify an arbitrary sandbox script's routing coverage.
export const WorkspaceAIEgressActivity: FC<WorkspaceAIEgressActivityProps> = ({
	workspaceId,
}) => {
	const queryClient = useQueryClient();
	const sessionsQuery = useQuery(workspaceAISandboxSessions(workspaceId));
	const eventsQuery = useInfiniteQuery(
		infiniteWorkspaceAISandboxNetworkEvents(workspaceId),
	);
	const sessions = sessionsQuery.data ?? [];
	const events = eventsQuery.data?.pages.flat() ?? [];
	const sessionsByID = new Map(
		sessions.map((session) => [session.id, session]),
	);

	useEffect(() => {
		const reload = () => {
			void Promise.all([
				queryClient.invalidateQueries({
					queryKey: workspaceAISandboxSessionsKey(workspaceId),
				}),
				queryClient.invalidateQueries({
					queryKey: workspaceAISandboxActivityKey(workspaceId),
				}),
			]);
		};
		return createReconnectingWebSocket({
			connect() {
				const socket = watchWorkspaceAISandboxActivity(workspaceId);
				socket.addEventListener("message", reload);
				return socket;
			},
			onOpen: reload,
		});
	}, [queryClient, workspaceId]);

	if (sessionsQuery.isLoading || eventsQuery.isLoading) {
		return (
			<section className="flex flex-col gap-4">
				<Header />
				<div className="flex justify-center p-6">
					<Spinner loading />
				</div>
			</section>
		);
	}

	const queryError = sessionsQuery.error ?? eventsQuery.error;
	if (queryError) {
		return (
			<section className="flex flex-col gap-4">
				<Header />
				<ErrorAlert error={queryError} />
			</section>
		);
	}

	if (events.length === 0) {
		return (
			<section className="flex flex-col gap-4">
				<Header />
				<EmptyState
					message={
						sessions.length === 0
							? "No confinement sessions yet"
							: "No egress attempts recorded"
					}
					description={
						sessions.length === 0
							? "Egress activity appears once a confined agent reports a session."
							: undefined
					}
				/>
			</section>
		);
	}

	return (
		<section className="flex flex-col gap-4">
			<Header />
			<div className="flex flex-col gap-3 rounded-lg border border-solid border-border p-4">
				<Table aria-label="AI egress network events">
					<TableHeader>
						<TableRow>
							<TableHead>Time</TableHead>
							<TableHead>Protocol</TableHead>
							<TableHead>Destination</TableHead>
							<TableHead>Action</TableHead>
							<TableHead>Enforcement</TableHead>
							<TableHead>Policy</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{events.map((event) => {
							const enforcement = sessionsByID.get(
								event.session_id,
							)?.egress_enforcement;
							return (
								<TableRow key={event.id}>
									<TableCell>
										{formatDate(new Date(event.occurred_at))}
									</TableCell>
									<TableCell>{event.protocol}</TableCell>
									<TableCell>
										{event.host}:{event.port}
									</TableCell>
									<TableCell>
										<Badge
											variant={
												event.action === "denied" ? "destructive" : "default"
											}
											size="xs"
										>
											{event.action}
										</Badge>
									</TableCell>
									<TableCell>
										{enforcement ? (
											<Badge
												variant={enforcementVariant[enforcement]}
												size="xs"
												title={enforcementHelp[enforcement]}
											>
												{enforcement}
											</Badge>
										) : (
											"Unknown"
										)}
									</TableCell>
									<TableCell>{event.policy_revision}</TableCell>
								</TableRow>
							);
						})}
					</TableBody>
				</Table>

				<div className="flex items-center gap-3">
					<Spinner loading={eventsQuery.isFetchingNextPage} size="sm" />
					{eventsQuery.hasNextPage && (
						<Button
							variant="outline"
							size="sm"
							disabled={eventsQuery.isFetchingNextPage}
							onClick={() => eventsQuery.fetchNextPage()}
						>
							Load more
						</Button>
					)}
				</div>
			</div>
		</section>
	);
};

const Header: FC = () => (
	<div className="flex flex-col gap-1">
		<h3 className="text-base font-semibold text-content-primary m-0">
			AI egress activity
		</h3>
		<span className="text-sm text-content-secondary">
			Network egress from AI-confined execution is default-deny. Every allowed
			and denied destination is recorded here.
		</span>
	</div>
);
