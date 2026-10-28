import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link } from "react-router";
import {
	createFoundationRun,
	foundationPath,
	foundationPermissions,
	templateFoundation,
} from "#/api/queries/foundations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { FoundationImpact } from "#/pages/FoundationsPage/FoundationImpact";

type FoundationTemplateDeleteDialogProps = {
	templateId: string;
	templateName: string;
	onConfirm: () => void;
	onCancel: () => void;
};

/** Cleanup and template deletion are separate, explicitly confirmed operations. */
export const FoundationTemplateDeleteDialog: React.FC<
	FoundationTemplateDeleteDialogProps
> = ({ templateId, templateName, onConfirm, onCancel }) => {
	const client = useQueryClient();
	const query = useQuery(templateFoundation(templateId));
	const resources = query.data;
	const permissions = useQuery({
		...foundationPermissions(resources?.organization_id ?? "", resources?.id),
		enabled: Boolean(resources),
	});
	const run = useMutation(
		createFoundationRun(
			client,
			resources?.organization_id ?? "",
			resources?.id ?? "",
		),
	);
	const running =
		run.isPending ||
		resources?.status === "pending" ||
		resources?.runs.some((item) =>
			["pending", "running", "canceling"].includes(item.job.status),
		);
	const cleaned =
		resources === null ||
		(resources &&
			!running &&
			(resources.status === "destroyed" ||
				(!resources.checkpoint_revision_id &&
					resources.status === "unconfigured")));
	if (cleaned && !query.error)
		return (
			<DeleteDialog
				isOpen
				entity="template"
				name={templateName}
				onConfirm={onConfirm}
				onCancel={onCancel}
				info={
					resources
						? "Foundation infrastructure is cleaned up. Deleting this template is a separate step. The Foundation and its retained history remain in the organization."
						: undefined
				}
			/>
		);
	const latest = resources?.runs.toSorted((a, b) =>
		b.created_at.localeCompare(a.created_at),
	)[0];
	return (
		<ConfirmDialog
			open
			title="Clean up Foundation before deleting template"
			type="delete"
			confirmText="Destroy Foundation infrastructure"
			confirmLoading={Boolean(running)}
			disabled={
				Boolean(query.error) ||
				!resources?.checkpoint_revision_id ||
				resources.status === "unknown" ||
				!permissions.data?.canUseFoundation ||
				Boolean(running)
			}
			onClose={onCancel}
			onConfirm={() => {
				if (
					resources?.checkpoint_revision_id &&
					permissions.data?.canUseFoundation &&
					!running &&
					resources.status !== "unknown"
				)
					run.mutate({
						revision_id: resources.checkpoint_revision_id,
						action: "destroy",
					});
			}}
			description={
				<div className="space-y-3">
					<p>
						This destroys live shared infrastructure. Cleanup must succeed
						before you can separately confirm template deletion. This is not
						atomic: if deletion fails, infrastructure remains destroyed. Saved
						Foundation revisions and job history are retained.
					</p>
					{query.isLoading && <p>Checking the attached Foundation…</p>}
					{resources && (
						<>
							<p>
								{resources.name}: {resources.status}
							</p>
							<p>
								{resources.prevent_destroy_with_workspaces
									? "Workspace destruction protection is enabled. The server blocks Destroy while consumers exist."
									: "Workspace destruction protection is disabled. Destroy can break running and stopped consumers."}
							</p>
							<FoundationImpact resources={resources} />
							<Link
								to={foundationPath(resources.organization_id, resources.id)}
							>
								Inspect Foundation state and logs
							</Link>
						</>
					)}
					{running && (
						<p>
							Waiting for Foundation cleanup. Do not delete its Terraform state.
						</p>
					)}
					{resources?.status === "unknown" && (
						<p>
							State is unknown. Manual reconciliation is required before cleanup
							can continue.
						</p>
					)}
					{permissions.data && !permissions.data.canUseFoundation && (
						<p>
							Foundation use permission is required to destroy infrastructure.
						</p>
					)}
					{latest?.job.error && <ErrorAlert error={latest.job.error} />}
					{Boolean(query.error || permissions.error || run.error) && (
						<ErrorAlert error={query.error || permissions.error || run.error} />
					)}
					{Boolean(query.error || permissions.error) && (
						<Button
							variant="outline"
							onClick={() => {
								void query.refetch();
								if (resources) void permissions.refetch();
							}}
						>
							Retry cleanup status
						</Button>
					)}
				</div>
			}
		/>
	);
};
