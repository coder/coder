import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { deleteFoundation } from "#/api/queries/foundations";
import type { Foundation } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

type FoundationDeleteProps = { resources: Foundation; canDelete: boolean };

/** Removes the Foundation separately from destroying its infrastructure. */
export const FoundationDelete: React.FC<FoundationDeleteProps> = ({
	resources,
	canDelete,
}) => {
	const [confirm, setConfirm] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);
	const section = useRef<HTMLElement>(null);
	const client = useQueryClient();
	const navigate = useNavigate();
	const mutation = useMutation(
		deleteFoundation(client, resources.organization_id, resources.id),
	);
	const active = resources.runs.some((run) =>
		["pending", "running", "canceling"].includes(run.job.status),
	);
	const cleaned =
		!active &&
		!resources.applied_revision_id &&
		(resources.status === "destroyed" || resources.status === "unconfigured");
	const disabled = !canDelete || !cleaned || mutation.isPending;
	return (
		<section
			ref={section}
			tabIndex={-1}
			className="space-y-3"
			aria-label="Delete Foundation"
		>
			<h2 className="text-xl font-semibold">Delete Foundation</h2>
			<p className="text-sm text-content-secondary">
				Destroy infrastructure first. Deleting the Foundation is a separate
				action that clears its template association; it does not destroy
				infrastructure or delete consumer workspaces.
			</p>
			{!canDelete && <p>Foundation delete permission is required.</p>}
			{!cleaned && <p>Infrastructure cleanup must finish before deletion.</p>}
			<Button
				ref={trigger}
				variant="destructive"
				disabled={disabled}
				onClick={() => setConfirm(true)}
			>
				Delete Foundation
			</Button>
			<ConfirmDialog
				open={confirm}
				type="delete"
				title="Delete Foundation?"
				confirmText="Confirm deletion"
				confirmLoading={mutation.isPending}
				disabled={disabled}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (trigger.current?.disabled) section.current?.focus();
					else trigger.current?.focus();
				}}
				onClose={() => {
					if (!mutation.isPending) setConfirm(false);
				}}
				onConfirm={() => {
					if (!disabled)
						mutation.mutate(undefined, {
							onSuccess: () => void navigate("/foundations"),
						});
				}}
				description={
					<div className="space-y-3">
						<p>
							Delete {resources.name} and clear its template association? Future
							builds will no longer receive outputs through this association.
							Historical build inputs remain retained. To keep the Foundation
							and recreate its infrastructure instead, cancel and use Apply.
						</p>
						{Boolean(mutation.error) && <ErrorAlert error={mutation.error} />}
					</div>
				}
			/>
		</section>
	);
};
