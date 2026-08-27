import type { FC } from "react";
import type { MCPGatewayEscalation } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { timeFrom } from "#/utils/time";

interface MCPEscalationsPageViewProps {
	escalations: readonly MCPGatewayEscalation[];
	isLoading: boolean;
	queryError: unknown;
	mutationError: unknown;
	approvingId?: string;
	denyingId?: string;
	referenceDate: Date;
	onApprove: (id: string) => void;
	onDeny: (id: string) => void;
}

export const MCPEscalationsPageView: FC<MCPEscalationsPageViewProps> = ({
	escalations,
	isLoading,
	queryError,
	mutationError,
	approvingId,
	denyingId,
	referenceDate,
	onApprove,
	onDeny,
}) => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>Tool call approvals</PageHeaderTitle>
				<PageHeaderSubtitle>
					Review MCP tool calls requested by Coder Agents before they run.
				</PageHeaderSubtitle>
			</PageHeader>

			<div className="flex flex-col gap-4">
				{Boolean(queryError) && <ErrorAlert error={queryError} />}
				{Boolean(mutationError) && <ErrorAlert error={mutationError} />}

				{isLoading ? (
					<div
						className="flex min-h-64 items-center justify-center"
						role="status"
						aria-label="Loading tool call approvals"
					>
						<Spinner loading />
					</div>
				) : queryError ? null : escalations.length === 0 ? (
					<EmptyState message="No tool calls are waiting for approval" />
				) : (
					<div
						className="flex flex-col gap-4"
						role="list"
						aria-label="Pending tool call approvals"
					>
						{escalations.map((escalation) => (
							<EscalationCard
								key={escalation.id}
								escalation={escalation}
								isApproving={approvingId === escalation.id}
								isDenying={denyingId === escalation.id}
								referenceDate={referenceDate}
								onApprove={onApprove}
								onDeny={onDeny}
							/>
						))}
					</div>
				)}
			</div>
		</Margins>
	);
};

interface EscalationCardProps {
	escalation: MCPGatewayEscalation;
	isApproving: boolean;
	isDenying: boolean;
	referenceDate: Date;
	onApprove: (id: string) => void;
	onDeny: (id: string) => void;
}

const EscalationCard: FC<EscalationCardProps> = ({
	escalation,
	isApproving,
	isDenying,
	referenceDate,
	onApprove,
	onDeny,
}) => {
	const titleId = `mcp-escalation-${escalation.id}`;
	const isResolving = isApproving || isDenying;

	return (
		<article
			className="rounded-lg border border-border-default border-solid bg-surface-primary p-5"
			role="listitem"
			aria-labelledby={titleId}
		>
			<div className="flex flex-col gap-5">
				<div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
					<div className="min-w-0">
						<div className="flex flex-wrap items-center gap-2">
							<h2
								id={titleId}
								className="m-0 break-words font-mono text-base font-semibold text-content-primary"
							>
								{escalation.tool}
							</h2>
							<Badge variant="warning" size="sm">
								Pending
							</Badge>
						</div>
						<p className="mb-0 mt-2 text-sm text-content-secondary">
							Requested {timeFrom(escalation.created_at, referenceDate)}.
							Expires {timeFrom(escalation.expires_at, referenceDate)}.
						</p>
					</div>

					<div className="flex shrink-0 gap-2">
						<Button
							variant="destructive"
							size="sm"
							disabled={isResolving}
							aria-label={`Deny ${escalation.tool} tool call`}
							onClick={() => onDeny(escalation.id)}
						>
							<Spinner loading={isDenying}>Deny</Spinner>
						</Button>
						<Button
							size="sm"
							disabled={isResolving}
							aria-label={`Approve ${escalation.tool} tool call`}
							onClick={() => onApprove(escalation.id)}
						>
							<Spinner loading={isApproving}>Approve</Spinner>
						</Button>
					</div>
				</div>

				<dl className="m-0 grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
					<div className="min-w-0">
						<dt className="text-xs font-medium text-content-secondary">
							Server
						</dt>
						<dd className="m-0 mt-1 break-words font-mono text-content-primary">
							{escalation.server_slug || "Unknown server"}
						</dd>
					</div>
					<div className="min-w-0">
						<dt className="text-xs font-medium text-content-secondary">
							Workspace
						</dt>
						<dd className="m-0 mt-1 break-words text-content-primary">
							{escalation.workspace_name || "Unknown workspace"}
						</dd>
					</div>
				</dl>

				<details className="max-w-full rounded-md border border-border border-solid bg-surface-secondary p-3">
					<summary className="cursor-pointer text-sm font-medium text-content-primary">
						Arguments
					</summary>
					<div className="mt-3 max-w-full overflow-x-auto">
						<pre className="m-0 w-max min-w-full whitespace-pre-wrap break-words font-mono text-xs text-content-secondary">
							{escalation.input}
						</pre>
					</div>
				</details>
			</div>
		</article>
	);
};
