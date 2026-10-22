import { useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import {
	createNetworkPolicyRevision,
	createNetworkPolicyTarget,
	deleteNetworkPolicy,
	publishNetworkPolicy,
	setNetworkPolicyTemplates,
} from "#/api/queries/networkPolicies";
import type { NetworkPolicy, Template } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { NetworkPolicyAttachments } from "./NetworkPolicyAttachments";
import { NetworkPolicyDelete } from "./NetworkPolicyDelete";
import { NetworkPolicyEditor } from "./NetworkPolicyEditor";
import { NetworkPolicyPublications } from "./NetworkPolicyPublications";
import { NetworkPolicyRunLogs } from "./NetworkPolicyRunLogs";
import { NetworkPolicyTargetForm } from "./NetworkPolicyTargetForm";
import { latestNetworkPolicyRevision } from "./networkPolicyRevisions";
import { isNetworkPolicyTargetBlocked } from "./networkPolicyTargets";

type NetworkPolicyDetailProps = {
	organization: string;
	policy: NetworkPolicy;
	templates: readonly Template[];
};

/** Edits persisted policy revisions and coordinates independent publication jobs. */
export const NetworkPolicyDetail: React.FC<NetworkPolicyDetailProps> = ({
	organization,
	policy,
	templates,
}) => {
	const queryClient = useQueryClient();
	const [openRuns, setOpenRuns] = useState<string[]>([]);
	const [selectedRevisionId, setSelectedRevisionId] = useState(
		() => latestNetworkPolicyRevision(policy.revisions)?.id ?? "",
	);
	const save = useMutation(
		createNetworkPolicyRevision(queryClient, organization, policy.id),
	);
	const attach = useMutation(
		setNetworkPolicyTemplates(queryClient, organization, policy.id),
	);
	const target = useMutation(
		createNetworkPolicyTarget(queryClient, organization, policy.id),
	);
	const publish = useMutation(
		publishNetworkPolicy(queryClient, organization, policy.id),
	);
	const remove = useMutation(
		deleteNetworkPolicy(queryClient, organization, policy.id),
	);
	const mutationPending =
		save.isPending ||
		attach.isPending ||
		target.isPending ||
		publish.isPending ||
		remove.isPending;
	const isPending = mutationPending || Boolean(policy.deletion_requested_at);
	const deleteBlockedReason = policy.targets.some(
		(target) => target.status === "unknown",
	)
		? "Deletion is blocked because a previous Terraform operation has an unknown outcome. Recovery is not implemented in this POC. The policy and logs are retained to avoid losing track of managed resources."
		: policy.targets.some(isNetworkPolicyTargetBlocked)
			? "Wait for active target operations to finish before deleting."
			: undefined;
	return (
		<div className="space-y-10">
			{policy.deletion_requested_at && (
				<p
					role="status"
					className="rounded-lg border border-solid border-border p-4 text-content-secondary"
				>
					{policy.deleted_at
						? "Policy deleted. Managed resources have been removed from every target. Versions, state, and logs are retained for inspection."
						: "Deletion requested. Terraform cleanup must complete on every target. Editing and new workspace starts are blocked while deletion is incomplete. Inspect target status and logs below; retry deletion only after safe failures."}
				</p>
			)}
			<NetworkPolicyEditor
				policy={policy}
				selectedRevisionId={selectedRevisionId}
				onSelectRevision={setSelectedRevisionId}
				isPending={mutationPending}
				readOnly={Boolean(policy.deletion_requested_at)}
				error={save.error}
				onSave={(request, onSuccess) => save.mutate(request, { onSuccess })}
			/>
			<NetworkPolicyAttachments
				policy={policy}
				templates={templates}
				isPending={isPending}
				error={attach.error}
				onSubmit={(request, onSuccess) => attach.mutate(request, { onSuccess })}
			/>
			<NetworkPolicyPublications
				policy={policy}
				selectedRevisionId={selectedRevisionId}
				isPending={isPending}
				error={publish.error}
				onPublish={(request, onSuccess) =>
					publish.mutate(request, { onSuccess })
				}
			/>
			<section className="space-y-4" aria-label="Add deployment target">
				<h2 className="text-xl font-semibold">Add target</h2>
				<NetworkPolicyTargetForm
					generation={policy.generation}
					isPending={isPending}
					error={target.error}
					onSubmit={(request, onSuccess) =>
						target.mutate(request, { onSuccess })
					}
				/>
			</section>
			<NetworkPolicyDelete
				name={policy.name}
				generation={policy.generation}
				isPending={mutationPending}
				deleted={Boolean(policy.deleted_at)}
				blockedReason={deleteBlockedReason}
				error={remove.error}
				onDelete={(request, onSuccess) => remove.mutate(request, { onSuccess })}
			/>
			<section className="space-y-4" aria-label="Publication history">
				<h2 className="text-xl font-semibold">Runs and plans</h2>
				{policy.runs.length === 0 ? (
					<p className="text-content-secondary">
						No runs yet. Preview a plan or publish a saved version.
					</p>
				) : (
					policy.runs
						.toSorted((a, b) => b.created_at.localeCompare(a.created_at))
						.map((run) => (
							<details
								key={run.id}
								onToggle={(event) => {
									const open = event.currentTarget.open;
									setOpenRuns((ids) =>
										open
											? [...new Set([...ids, run.id])]
											: ids.filter((id) => id !== run.id),
									);
								}}
								className="rounded-lg border border-solid border-border p-4"
							>
								<summary className="cursor-pointer">
									{run.operation} on{" "}
									{policy.targets.find((target) => target.id === run.target_id)
										?.name || run.target_id}{" "}
									<Badge>{run.status || "Unknown"}</Badge>
								</summary>
								<div className="mt-4 space-y-3">
									<p className="text-sm text-content-secondary">
										Job: {run.job_id} · Version: {run.revision_id.slice(0, 8)}
									</p>
									{run.error && (
										<p role="alert" className="text-content-destructive">
											{run.error}
										</p>
									)}
									<h3 className="text-sm font-semibold">Plan / drift report</h3>
									<pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded bg-surface-secondary p-4 text-xs">
										{run.plan || "No plan available yet."}
									</pre>
									{openRuns.includes(run.id) && (
										<NetworkPolicyRunLogs
											organization={organization}
											policyId={policy.id}
											runId={run.id}
										/>
									)}
								</div>
							</details>
						))
				)}
			</section>
		</div>
	);
};
