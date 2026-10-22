import { useRef, useState } from "react";
import type { DeleteNetworkPolicyRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

type NetworkPolicyDeleteProps = {
	name: string;
	generation: number;
	isPending: boolean;
	deleted: boolean;
	blockedReason: string | undefined;
	error: unknown;
	onDelete: (
		request: DeleteNetworkPolicyRequest,
		onSuccess: () => void,
	) => void;
};

/** Confirms removal of a policy and its Terraform-managed network rules. */
export const NetworkPolicyDelete: React.FC<NetworkPolicyDeleteProps> = ({
	name,
	generation,
	isPending,
	deleted,
	blockedReason,
	error,
	onDelete,
}) => {
	const [confirmationGeneration, setConfirmationGeneration] =
		useState<number>();
	const open = confirmationGeneration !== undefined;
	const changed = open && confirmationGeneration !== generation;
	const trigger = useRef<HTMLButtonElement>(null);
	const section = useRef<HTMLElement>(null);
	return (
		<section
			ref={section}
			tabIndex={-1}
			className="space-y-4"
			aria-label="Policy deletion"
		>
			<h2 className="text-xl font-semibold">
				{deleted ? "Policy deleted" : "Delete policy"}
			</h2>
			<p className="text-sm text-content-secondary">
				{deleted
					? "Terraform cleanup is complete. The policy's retained versions and logs remain available for inspection."
					: "Deleting removes this policy's managed network rules through Terraform. Other cluster policies and workspace resources are not deleted."}
			</p>
			{!deleted && blockedReason && (
				<p className="text-sm text-content-secondary">{blockedReason}</p>
			)}
			{!deleted && Boolean(error) && !open && <ErrorAlert error={error} />}
			{!deleted && (
				<Button
					ref={trigger}
					variant="destructive"
					disabled={isPending || Boolean(blockedReason)}
					onClick={() => setConfirmationGeneration(generation)}
				>
					Delete policy
				</Button>
			)}
			<ConfirmDialog
				type="delete"
				title={`Delete ${name}?`}
				open={open && !deleted}
				confirmLoading={isPending}
				disabled={isPending || Boolean(blockedReason) || changed}
				onClose={() => {
					if (!isPending) setConfirmationGeneration(undefined);
				}}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (!trigger.current || trigger.current.disabled)
						section.current?.focus();
					else trigger.current?.focus();
				}}
				onConfirm={() => {
					if (confirmationGeneration !== undefined)
						onDelete({ expected_generation: confirmationGeneration }, () =>
							setConfirmationGeneration(undefined),
						);
				}}
				description={
					<div className="space-y-4">
						<p>
							This can restore network access for running workspaces. Terraform
							cleanup must succeed on every target before deletion completes.
							Failed or unknown cleanup keeps the policy and its logs available.
						</p>
						{changed && (
							<p>
								The policy changed. Close this dialog and review it before
								deleting.
							</p>
						)}
						{blockedReason && <p>{blockedReason}</p>}
						{Boolean(error) && <ErrorAlert error={error} />}
					</div>
				}
			/>
		</section>
	);
};
