import { ChevronLeftIcon, PlayIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	Link,
	useBeforeUnload,
	unstable_usePrompt as usePrompt,
	useSearchParams,
} from "react-router";
import { templateArchive, uploadFile } from "#/api/queries/files";
import {
	createFoundationRevision,
	createFoundationRun,
	foundationPath,
} from "#/api/queries/foundations";
import type {
	Foundation,
	TemplateResourceRevision,
	TemplateResourceRun,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	Topbar,
	TopbarButton,
	TopbarData,
	TopbarIconButton,
} from "#/components/FullPageLayout/Topbar";
import { Loader } from "#/components/Loader/Loader";
import { FoundationImpact } from "#/pages/FoundationsPage/FoundationImpact";
import {
	FoundationPlanPreview,
	getFoundationPlanPreview,
} from "#/pages/FoundationsPage/FoundationPlanPreview";
import { SharedResourceRunLogs } from "#/pages/TemplatePage/TemplateSharedResourcesPage/SharedResourceRunLogs";
import { existsFile, type FileTree, isFolder } from "#/utils/filetree";
import { TarReader } from "#/utils/tar";
import { writeTemplateArchive } from "#/utils/templateArchive";
import { TemplateFileEditor } from "./TemplateFileEditor";
import { findEntrypointFile } from "./TemplateVersionEditorPage";

const runActionLabels: Record<string, string | undefined> = {
	plan: "Plan",
	apply: "Apply",
	destroy: "Destroy",
};

type ResourceArchiveProps = {
	resources: Foundation;
	canUpdate: boolean;
	canUse: boolean;
};

/** Edits a Foundation archive without loading a workspace template version. */
export const SharedResourceFilesEditor: React.FC<ResourceArchiveProps> = ({
	resources,
	canUpdate,
	canUse,
}) => {
	// Freeze the opened archive. Polling revision history must not replace a draft.
	const [revision] = useState(
		() =>
			resources.revisions.find(
				(item) => item.id === resources.current_revision_id,
			) ?? resources.revisions[0],
	);
	const query = useQuery({
		...templateArchive(revision?.file_id ?? ""),
		enabled: Boolean(revision),
	});
	const [emptyArchive] = useState(() => ({
		archive: new TarReader(),
		files: { "main.tf": "" },
	}));
	const initial = revision ? query.data : emptyArchive;
	return (
		<>
			{Boolean(query.error) && (
				<div className="p-3">
					<ErrorAlert error={query.error} />
					<Button variant="outline" onClick={() => void query.refetch()}>
						Retry files
					</Button>
				</div>
			)}
			{initial ? (
				<ResourceEditorSession
					resources={resources}
					canUpdate={canUpdate}
					canUse={canUse}
					initial={initial}
					revision={revision}
				/>
			) : (
				<>
					<ResourceEditorHeader resources={resources} />
					{query.isLoading && <Loader />}
				</>
			)}
		</>
	);
};

type ResourceEditorHeaderProps = {
	resources: Foundation;
	children?: React.ReactNode;
};

const ResourceEditorHeader: React.FC<ResourceEditorHeaderProps> = ({
	resources,
	children,
}) => {
	return (
		<Topbar className="flex items-center gap-4 pr-4 shrink-0">
			<TopbarIconButton asChild>
				<Link
					to={foundationPath(resources.organization_id, resources.id)}
					aria-label="Back to Foundation"
				>
					<ChevronLeftIcon />
				</Link>
			</TopbarIconButton>
			<TopbarData>{resources.name}</TopbarData>
			<div className="ml-auto flex gap-2">{children}</div>
		</Topbar>
	);
};

type ResourceEditorSessionProps = {
	resources: Foundation;
	canUpdate: boolean;
	canUse: boolean;
	initial: { archive: TarReader; files: FileTree };
	revision: TemplateResourceRevision | undefined;
};

const ResourceEditorSession: React.FC<ResourceEditorSessionProps> = ({
	resources,
	canUpdate,
	canUse,
	initial,
	revision,
}) => {
	const queryClient = useQueryClient();
	const [files, setFiles] = useState(initial.files);
	const [savedFiles, setSavedFiles] = useState(initial.files);
	const [savedRevision, setSavedRevision] = useState(revision);
	const [selectedRun, setSelectedRun] = useState<
		TemplateResourceRun | undefined
	>(() => {
		const runs = resources.runs.toSorted((a, b) =>
			b.created_at.localeCompare(a.created_at),
		);
		return runs.find((item) => item.revision_id === revision?.id) ?? runs[0];
	});
	const [saveError, setSaveError] = useState<unknown>();
	const [saving, setSaving] = useState(false);
	const [confirmApply, setConfirmApply] = useState(false);
	const applyTrigger = useRef<HTMLButtonElement>(null);
	const output = useRef<HTMLElement>(null);
	const [params, setParams] = useSearchParams();
	const upload = useMutation(uploadFile());
	const save = useMutation(
		createFoundationRevision(
			queryClient,
			resources.organization_id,
			resources.id,
		),
	);
	const run = useMutation(
		createFoundationRun(queryClient, resources.organization_id, resources.id),
	);
	const dirty = files !== savedFiles;
	const liveRun =
		selectedRun &&
		(resources.runs.find((item) => item.id === selectedRun.id) ?? selectedRun);
	const busy =
		run.isPending ||
		resources.status === "pending" ||
		resources.runs.some((item) =>
			["pending", "running", "canceling"].includes(item.job.status),
		) ||
		Boolean(
			liveRun &&
				["pending", "running", "canceling"].includes(liveRun.job.status),
		);
	const blocked = busy || resources.status === "unknown";
	const canRun = !blocked && !saving && !dirty && Boolean(savedRevision);
	const canApply = canUse && canRun;

	let statusMessage =
		"Save files independently, then Plan to preview or Apply to change infrastructure. Apply always creates a fresh plan, which may differ from an earlier preview.";
	if (busy) {
		statusMessage =
			"Waiting for the current operation. You can still edit and save files. Logs refresh automatically.";
	} else if (resources.status === "unknown") {
		statusMessage =
			"Foundation state is unknown. You can edit and save files, but must reconcile state before Plan or Apply.";
	} else if (dirty) {
		statusMessage = "Unsaved edits. Save files before Plan or Apply.";
	}
	const requestedPath = params.get("path");
	const activePath =
		requestedPath &&
		existsFile(requestedPath, files) &&
		!isFolder(requestedPath, files)
			? requestedPath
			: findEntrypointFile(files);
	const saveFiles = useCallback(async () => {
		if (saving || !canUpdate) return;
		setSaving(true);
		setSaveError(undefined);
		try {
			const archive = await writeTemplateArchive(initial.archive, files);
			const uploaded = await upload.mutateAsync(archive);
			const revision = await save.mutateAsync({ file_id: uploaded.hash });
			setSavedFiles(files);
			setSavedRevision(revision);
		} catch (error) {
			setSaveError(error);
		} finally {
			setSaving(false);
		}
	}, [saving, canUpdate, files, initial.archive, upload, save]);

	useEffect(() => {
		const listener = (event: KeyboardEvent) => {
			if (!(event.ctrlKey || event.metaKey)) return;
			if (event.key === "s") {
				event.preventDefault();
				void saveFiles();
			}
		};
		document.addEventListener("keydown", listener);
		return () => document.removeEventListener("keydown", listener);
	}, [saveFiles]);
	usePrompt({
		message:
			"You have unsaved shared resource changes. Are you sure you want to leave?",
		when: ({ currentLocation, nextLocation }) =>
			(dirty || saving) &&
			(currentLocation.pathname !== nextLocation.pathname ||
				new URLSearchParams(nextLocation.search).get("edit") !== "true"),
	});
	useBeforeUnload(
		useCallback(
			(event: BeforeUnloadEvent) => {
				if (dirty || saving) event.preventDefault();
			},
			[dirty, saving],
		),
	);

	return (
		<>
			<ResourceEditorHeader resources={resources}>
				<TopbarButton
					disabled={saving || !canUpdate}
					title="Save files (Ctrl + S)"
					onClick={() => void saveFiles()}
				>
					{saving ? "Saving…" : "Save files"}
				</TopbarButton>
				<TopbarButton
					disabled={!canRun || !canUpdate}
					onClick={() => {
						if (canRun && canUpdate && savedRevision)
							run.mutate(
								{ revision_id: savedRevision.id, action: "plan" },
								{ onSuccess: setSelectedRun },
							);
					}}
				>
					<PlayIcon />
					Plan
				</TopbarButton>
				<TopbarButton
					ref={applyTrigger}
					variant="default"
					disabled={!canApply}
					onClick={() => setConfirmApply(true)}
				>
					Apply
				</TopbarButton>
			</ResourceEditorHeader>
			<ConfirmDialog
				open={confirmApply}
				hideCancel={false}
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					if (applyTrigger.current?.disabled) output.current?.focus();
					else applyTrigger.current?.focus();
				}}
				title="Apply Foundation?"
				confirmText="Apply Foundation"
				confirmLoading={run.isPending}
				disabled={!canApply}
				onClose={() => {
					if (!run.isPending) setConfirmApply(false);
				}}
				onConfirm={() => {
					if (canApply && savedRevision)
						run.mutate(
							{ revision_id: savedRevision.id, action: "apply" },
							{
								onSuccess: (result) => {
									setSelectedRun(result);
									setConfirmApply(false);
								},
							},
						);
				}}
				description={
					<div className="space-y-3">
						<p>
							Apply revision {savedRevision?.id} using a fresh plan, which may
							differ from any earlier Plan preview. It can create, change, or
							delete infrastructure, even when workspace destruction protection
							is enabled.
						</p>
						<FoundationImpact resources={resources} />
						<FoundationPlanPreview
							preview={getFoundationPlanPreview(resources, savedRevision?.id)}
						/>
						{Boolean(run.error) && <ErrorAlert error={run.error} />}
					</div>
				}
			/>
			<div className="flex flex-1 min-h-0 overflow-hidden">
				<TemplateFileEditor
					fileTree={files}
					onFileTreeChange={setFiles}
					activePath={activePath}
					onActivePathChange={(path) => {
						const next = new URLSearchParams(params);
						if (path) next.set("path", path);
						else next.delete("path");
						setParams(next, { replace: true });
					}}
				>
					<section
						ref={output}
						tabIndex={-1}
						aria-label="Terraform output"
						className="p-4 space-y-3"
					>
						<h2 className="text-sm font-semibold m-0">Output</h2>
						<p className="text-sm text-content-secondary break-all">
							Foundation {resources.name} · {resources.id}
						</p>
						{!canUpdate && (
							<p>
								Read-only: Foundation update permission is required to save or
								plan.
							</p>
						)}
						{!canUse && <p>Foundation use permission is required to apply.</p>}
						<p className="text-sm text-content-secondary m-0">
							{statusMessage}
						</p>
						{Boolean(saveError || run.error) && (
							<ErrorAlert error={saveError || run.error} />
						)}
						{savedRevision && (
							<p className="text-sm text-content-secondary">
								Saved revision {savedRevision.id.slice(0, 8)}. Saving does not
								run Terraform.
							</p>
						)}
						{liveRun && (
							<>
								<p className="text-sm">
									{runActionLabels[liveRun.action] ?? liveRun.action}:{" "}
									{liveRun.job.status} · Revision{" "}
									{liveRun.revision_id.slice(0, 8)}
									{liveRun.revision_id !== savedRevision?.id &&
										" (not the editor's saved revision)"}
								</p>
								{liveRun.job.error && <ErrorAlert error={liveRun.job.error} />}
								<SharedResourceRunLogs
									organization={resources.organization_id}
									foundationId={resources.id}
									runId={liveRun.id}
								/>
							</>
						)}
					</section>
				</TemplateFileEditor>
			</div>
		</>
	);
};
