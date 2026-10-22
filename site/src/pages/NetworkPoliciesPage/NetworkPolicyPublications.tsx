import { useRef, useState } from "react";
import type {
	NetworkPolicy,
	PublishNetworkPolicyRequest,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { NetworkPolicyTargetRow } from "./NetworkPolicyTargetRow";
import { latestNetworkPolicyRevision } from "./networkPolicyRevisions";
import { isNetworkPolicyTargetBlocked } from "./networkPolicyTargets";

type NetworkPolicyPublicationsProps = {
	policy: NetworkPolicy;
	selectedRevisionId: string;
	isPending: boolean;
	error: unknown;
	onPublish: (
		request: PublishNetworkPolicyRequest,
		onSuccess: () => void,
	) => void;
};

/** Publication always uses saved HCL and requires explicit target selection. */
export const NetworkPolicyPublications: React.FC<
	NetworkPolicyPublicationsProps
> = ({ policy, selectedRevisionId, isPending, error, onPublish }) => {
	const [selectedTargets, setSelectedTargets] = useState<string[]>([]);
	const [confirmation, setConfirmation] =
		useState<PublishNetworkPolicyRequest>();
	const triggerRef = useRef<HTMLButtonElement | null>(null);
	const sectionRef = useRef<HTMLElement>(null);
	const targetIds = selectedTargets.filter((id) =>
		policy.targets.some(
			(target) => target.id === id && !isNetworkPolicyTargetBlocked(target),
		),
	);
	const newest = latestNetworkPolicyRevision(policy.revisions);
	const isRollback = Boolean(
		selectedRevisionId && newest && selectedRevisionId !== newest.id,
	);
	return (
		<section
			ref={sectionRef}
			tabIndex={-1}
			className="space-y-4"
			aria-label="Policy deployment"
		>
			<h2 className="text-xl font-semibold">Targets and publication</h2>
			{Boolean(error) && <ErrorAlert error={error} />}
			{policy.targets.length === 0 ? (
				<p className="text-content-secondary">
					Add a target below before previewing or publishing.
				</p>
			) : (
				<Table aria-label="Deployment targets" className="min-w-2xl">
					<TableHeader>
						<TableRow>
							<TableHead>Select</TableHead>
							<TableHead>Target</TableHead>
							<TableHead>Status</TableHead>
							<TableHead>Desired / Applied version</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{policy.targets.map((target) => (
							<NetworkPolicyTargetRow
								key={target.id}
								target={target}
								selected={targetIds.includes(target.id)}
								disabled={isPending || isNetworkPolicyTargetBlocked(target)}
								onSelectedChange={(selected) =>
									setSelectedTargets(
										selected
											? [...targetIds, target.id]
											: targetIds.filter((id) => id !== target.id),
									)
								}
							/>
						))}
					</TableBody>
				</Table>
			)}
			<p className="text-sm text-content-secondary">
				Operations use the selected saved version, not unsaved HCL. Preview runs
				a Terraform plan and checks drift without changing applied state.
				Publishing queues real provisioner jobs; applied status changes only
				after successful completion.
			</p>
			<div className="flex flex-wrap gap-2">
				{(
					[
						{ operation: "plan", label: "Preview plan / drift" },
						{
							operation: "apply",
							label: isRollback
								? "Roll back to selected version"
								: "Publish selected version",
						},
					] as const
				).map(({ operation, label }) => (
					<Button
						key={operation}
						variant="outline"
						disabled={
							!selectedRevisionId || targetIds.length === 0 || isPending
						}
						onClick={(event) => {
							triggerRef.current = event.currentTarget;
							setConfirmation({
								operation,
								revision_id: selectedRevisionId,
								target_ids: targetIds,
								expected_generation: policy.generation,
							});
						}}
					>
						{label}
					</Button>
				))}
			</div>
			<ConfirmDialog
				open={Boolean(confirmation)}
				hideCancel={false}
				title={
					confirmation?.operation === "plan"
						? "Preview selected version?"
						: "Publish selected version?"
				}
				confirmText="Queue operation"
				confirmLoading={isPending}
				disabled={confirmation?.target_ids.some(
					(id) => !targetIds.includes(id),
				)}
				onClose={() => {
					if (!isPending) setConfirmation(undefined);
				}}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (triggerRef.current?.disabled) sectionRef.current?.focus();
					else triggerRef.current?.focus();
				}}
				onConfirm={() => {
					if (confirmation)
						onPublish(confirmation, () => setConfirmation(undefined));
				}}
				description={
					<div className="space-y-2">
						<p>
							This queues the selected saved version on these explicit targets:
						</p>
						<ul>
							{confirmation?.target_ids.map((id) => {
								const target = policy.targets.find(
									(target) => target.id === id,
								);
								return (
									<li key={id}>
										{target
											? `${target.name}: ${target.cluster} / ${target.context} / ${target.namespace}`
											: id}
									</li>
								);
							})}
						</ul>
						<p>Version: {confirmation?.revision_id}</p>
						{confirmation?.target_ids.some((id) => !targetIds.includes(id)) && (
							<p>
								One or more selected targets are no longer available. Close this
								dialog and review target status.
							</p>
						)}
						{Boolean(error) && <ErrorAlert error={error} />}
					</div>
				}
			/>
		</section>
	);
};
