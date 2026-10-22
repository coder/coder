import { Link } from "react-router";
import type { NetworkPolicy } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";

type NetworkPoliciesPageViewProps = {
	policies: readonly NetworkPolicy[] | undefined;
	error: unknown;
	isLoading: boolean;
	onRetry: () => void;
	basePath: string;
};

/** Retains the last loaded policies when a background refresh fails. */
export const NetworkPoliciesPageView: React.FC<
	NetworkPoliciesPageViewProps
> = ({ policies, error, isLoading, onRetry, basePath }) => (
	<div className="space-y-4">
		{Boolean(error) && (
			<>
				<ErrorAlert error={error} />
				<Button variant="outline" onClick={onRetry}>
					Retry
				</Button>
			</>
		)}
		{isLoading && <Loader />}
		{policies?.length === 0 && (
			<p className="rounded-lg border border-solid border-border p-8 text-content-secondary">
				No network policies yet. Create a policy, attach templates, and add
				explicit deployment targets.
			</p>
		)}
		{policies && policies.length > 0 && (
			<Table aria-label="Network policies">
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Templates</TableHead>
						<TableHead>Versions</TableHead>
						<TableHead>Targets</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{policies.map((policy) => (
						<TableRow key={policy.id}>
							<TableCell>
								<Link
									to={`${basePath}/${policy.id}`}
									className="text-content-link"
								>
									{policy.name}
								</Link>
								{policy.deletion_requested_at && (
									<Badge className="ml-2">Deleting</Badge>
								)}
								<p className="text-sm text-content-secondary">
									{policy.description || "No description"}
								</p>
							</TableCell>
							<TableCell>{policy.template_ids.length}</TableCell>
							<TableCell>{policy.revisions.length}</TableCell>
							<TableCell>
								<div className="flex flex-wrap gap-2">
									{policy.targets.length === 0
										? "No targets"
										: policy.targets.map((target) => (
												<Badge key={target.id}>
													{target.name}: {target.status || "Unknown"}
												</Badge>
											))}
								</div>
							</TableCell>
						</TableRow>
					))}
				</TableBody>
			</Table>
		)}
	</div>
);
