import { useId, useRef, useState } from "react";
import { Link } from "react-router";
import type {
	CreateTemplateResourceRunRequest,
	Foundation,
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
import { FoundationImpact } from "#/pages/FoundationsPage/FoundationImpact";
import {
	FoundationPlanPreview,
	getFoundationPlanPreview,
} from "#/pages/FoundationsPage/FoundationPlanPreview";

type SharedResourceEditorProps = {
	resources: Foundation;
	canUpdate: boolean;
	canUse: boolean;
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
	canUpdate,
	canUse,
	editFilesHref,
	isRunning,
	runError,
	onRun,
}) => {
	const id = useId();
	const section = useRef<HTMLElement>(null);
	const destroyTrigger = useRef<HTMLButtonElement>(null);
	const applyTrigger = useRef<HTMLButtonElement>(null);
	const [selectedRevisionId, setSelectedRevisionId] = useState<string>();
	const revisionId = selectedRevisionId ?? resources.current_revision_id;
	const [destroyRevisionId, setDestroyRevisionId] = useState<string>();
	const [applyRevisionId, setApplyRevisionId] = useState<string>();
	const busy =
		isRunning ||
		resources.status === "pending" ||
		resources.runs.some((run) =>
			["pending", "running", "canceling"].includes(run.job.status),
		);
	const runBlocked = busy || resources.status === "unknown";
	const cleanupRevisionId = resources.checkpoint_revision_id ?? undefined;

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
				Edit files. Saving creates an immutable archive without running
				Terraform. Plan previews the selected revision. Apply always creates a
				fresh plan and applies it to shared infrastructure.
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
					disabled={runBlocked || !revisionId || !canUpdate}
					onClick={() => {
						if (revisionId && canUpdate)
							onRun({ revision_id: revisionId, action: "plan" });
					}}
				>
					Plan
				</Button>
				<Button
					ref={applyTrigger}
					disabled={runBlocked || !revisionId || !canUse}
					onClick={() => {
						if (revisionId) setApplyRevisionId(revisionId);
					}}
				>
					Apply
				</Button>
				<Button
					ref={destroyTrigger}
					variant="destructive"
					disabled={
						runBlocked ||
						!canUse ||
						!cleanupRevisionId ||
						resources.status === "destroyed"
					}
					onClick={() => setDestroyRevisionId(cleanupRevisionId)}
				>
					Destroy
				</Button>
			</div>
			{!canUpdate && (
				<p>Foundation update permission is required to plan changes.</p>
			)}
			{!canUse && (
				<p>
					Foundation use permission is required to apply or destroy
					infrastructure.
				</p>
			)}
			<ConfirmDialog
				open={applyRevisionId !== undefined}
				hideCancel={false}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (applyTrigger.current?.disabled) section.current?.focus();
					else applyTrigger.current?.focus();
				}}
				title="Apply Foundation?"
				confirmText="Apply Foundation"
				confirmLoading={isRunning}
				disabled={runBlocked || !canUse || applyRevisionId !== revisionId}
				onClose={() => {
					if (!isRunning) setApplyRevisionId(undefined);
				}}
				onConfirm={() => {
					if (
						applyRevisionId &&
						canUse &&
						!runBlocked &&
						applyRevisionId === revisionId
					)
						onRun({ revision_id: applyRevisionId, action: "apply" }, () =>
							setApplyRevisionId(undefined),
						);
				}}
				description={
					<div className="space-y-3">
						<p>
							Apply revision {applyRevisionId} using a fresh plan, which may
							differ from any earlier Plan preview. Apply can create, change, or
							delete infrastructure, even when workspace destruction protection
							is enabled.
						</p>
						<FoundationPlanPreview
							preview={getFoundationPlanPreview(resources, applyRevisionId)}
						/>
						<FoundationImpact resources={resources} />
						{Boolean(runError) && <ErrorAlert error={runError} />}
					</div>
				}
			/>
			<ConfirmDialog
				type="delete"
				title="Destroy shared resources?"
				confirmText="Destroy shared resources"
				open={destroyRevisionId !== undefined}
				confirmLoading={isRunning}
				disabled={
					runBlocked || !canUse || destroyRevisionId !== cleanupRevisionId
				}
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
						canUse &&
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
							Terraform will destroy the shared infrastructure owned by
							Foundation {resources.name} using checkpoint revision{" "}
							{destroyRevisionId?.slice(0, 8)}. The Foundation, template
							association, revisions, and job history remain. Apply again to
							recreate infrastructure.
						</p>
						<p>
							{resources.prevent_destroy_with_workspaces
								? "Workspace destruction protection is enabled. The server blocks Destroy while consumers exist."
								: "Workspace destruction protection is disabled. Destroy can break running and stopped consumers."}
						</p>
						{destroyRevisionId !== cleanupRevisionId && (
							<p>
								The checkpoint revision changed. Close this dialog and review
								the current state.
							</p>
						)}
						<FoundationImpact resources={resources} />
						{Boolean(runError) && <ErrorAlert error={runError} />}
					</div>
				}
			/>
		</section>
	);
};
