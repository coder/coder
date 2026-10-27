import { useId, useRef, useState } from "react";
import { Link } from "react-router";
import type {
	CreateTemplateResourceRunRequest,
	TemplateResources,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

type SharedResourceEditorProps = {
	resources: TemplateResources;
	editFilesHref: string;
	isRunning: boolean;
	runError: unknown;
	onRun: (
		request: CreateTemplateResourceRunRequest,
		onSuccess?: () => void,
	) => void;
};

/** Runs immutable revisions; source changes belong in the full archive editor. */
export const SharedResourceEditor: React.FC<SharedResourceEditorProps> = ({
	resources,
	editFilesHref,
	isRunning,
	runError,
	onRun,
}) => {
	const id = useId();
	const section = useRef<HTMLElement>(null);
	const destroyTrigger = useRef<HTMLButtonElement>(null);
	const [selectedRevisionId, setSelectedRevisionId] = useState<string>();
	const revisionId = selectedRevisionId ?? resources.current_revision_id;
	const [destroyRevisionId, setDestroyRevisionId] = useState<string>();
	const busy =
		isRunning ||
		resources.status === "pending" ||
		resources.runs.some((run) =>
			["pending", "running", "canceling"].includes(run.job.status),
		);
	const runBlocked = busy || resources.status === "unknown";
	const cleanupRevisionId = resources.applied_revision_id ?? undefined;
	const revisions = resources.revisions.toSorted((a, b) =>
		b.created_at.localeCompare(a.created_at),
	);
	return (
		<section
			ref={section}
			tabIndex={-1}
			aria-label="Terraform configuration"
			className="space-y-4"
		>
			<h2 className="text-xl font-semibold">Terraform configuration</h2>
			<p className="text-sm text-content-secondary">
				Edit the complete shared Terraform root, including local modules, in
				Edit files. Build saves an immutable archive and plans it; Publish
				applies that successful build to shared resources only.
			</p>
			<Button asChild>
				<Link to={editFilesHref}>Edit files</Link>
			</Button>
			<div className="space-y-2">
				<Label htmlFor={`${id}-revision`}>Revision history</Label>
				<Select
					value={revisionId ?? ""}
					disabled={busy || revisions.length === 0}
					onValueChange={setSelectedRevisionId}
				>
					<SelectTrigger id={`${id}-revision`}>
						<SelectValue placeholder="No saved revisions" />
					</SelectTrigger>
					<SelectContent>
						{revisions.map((item) => (
							<SelectItem key={item.id} value={item.id}>
								{item.id.slice(0, 8)} ({item.created_at})
								{item.id === resources.current_revision_id ? " · current" : ""}
								{item.id === resources.applied_revision_id ? " · applied" : ""}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
			{Boolean(runError) && !destroyRevisionId && (
				<ErrorAlert error={runError} />
			)}
			{busy && (
				<p className="text-sm text-content-secondary">
					Waiting for the current operation to finish. Job status and logs
					refresh automatically.
				</p>
			)}
			<div className="flex flex-wrap gap-2">
				<Button
					variant="outline"
					disabled={runBlocked || !revisionId}
					onClick={() => {
						if (revisionId) onRun({ revision_id: revisionId, action: "plan" });
					}}
				>
					Plan
				</Button>
				<Button
					disabled={runBlocked || !revisionId}
					onClick={() => {
						if (revisionId) onRun({ revision_id: revisionId, action: "apply" });
					}}
				>
					Apply
				</Button>
				<Button
					ref={destroyTrigger}
					variant="destructive"
					disabled={
						runBlocked || !cleanupRevisionId || resources.status === "destroyed"
					}
					onClick={() => setDestroyRevisionId(cleanupRevisionId)}
				>
					Destroy
				</Button>
			</div>
			<ConfirmDialog
				type="delete"
				title="Destroy shared resources?"
				confirmText="Destroy shared resources"
				open={destroyRevisionId !== undefined}
				confirmLoading={isRunning}
				disabled={runBlocked || destroyRevisionId !== cleanupRevisionId}
				onClose={() => {
					if (!isRunning) setDestroyRevisionId(undefined);
				}}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (destroyTrigger.current?.disabled) section.current?.focus();
					else destroyTrigger.current?.focus();
				}}
				onConfirm={() => {
					if (
						destroyRevisionId &&
						!runBlocked &&
						destroyRevisionId === cleanupRevisionId
					)
						onRun({ revision_id: destroyRevisionId, action: "destroy" }, () =>
							setDestroyRevisionId(undefined),
						);
				}}
				description={
					<div className="space-y-4">
						<p>
							Terraform will destroy the shared resources in this template's
							shared resource state using revision{" "}
							{destroyRevisionId?.slice(0, 8)}. Delete all workspaces using this
							template before destroying its shared resources. Saved revisions
							and job logs are retained.
						</p>
						{destroyRevisionId !== cleanupRevisionId && (
							<p>
								The applied revision changed. Close this dialog and review the
								current state.
							</p>
						)}
						{Boolean(runError) && <ErrorAlert error={runError} />}
					</div>
				}
			/>
		</section>
	);
};
