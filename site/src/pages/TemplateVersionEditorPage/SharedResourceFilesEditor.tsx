import { ChevronLeftIcon, PlayIcon } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	Link,
	useBeforeUnload,
	unstable_usePrompt as usePrompt,
	useSearchParams,
} from "react-router";
import { templateArchive, uploadFile } from "#/api/queries/files";
import {
	createTemplateResourceRevision,
	createTemplateResourceRun,
	templateResources,
} from "#/api/queries/templateResources";
import type {
	Template,
	TemplateResourceRevision,
	TemplateResourceRun,
	TemplateResources,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Topbar,
	TopbarButton,
	TopbarData,
	TopbarIconButton,
} from "#/components/FullPageLayout/Topbar";
import { Loader } from "#/components/Loader/Loader";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { SharedResourceRunLogs } from "#/pages/TemplatePage/TemplateSharedResourcesPage/SharedResourceRunLogs";
import { existsFile, type FileTree, isFolder } from "#/utils/filetree";
import { TarReader } from "#/utils/tar";
import { writeTemplateArchive } from "#/utils/templateArchive";
import { ProvisioningScopeNavigation } from "./ProvisioningScopeNavigation";
import { TemplateFileEditor } from "./TemplateFileEditor";
import { findEntrypointFile } from "./TemplateVersionEditorPage";

const runActionLabels: Record<string, string | undefined> = {
	plan: "Build",
	apply: "Publish",
	destroy: "Destroy",
};

type SharedResourceFilesEditorProps = { template: Template };

/** Loads the shared provisioning scope independently of workspace versions. */
export const SharedResourceFilesEditor: React.FC<
	SharedResourceFilesEditorProps
> = ({ template }) => {
	const query = useQuery(templateResources(template.id));
	return (
		<>
			{Boolean(query.error) && (
				<div className="p-3">
					<ErrorAlert error={query.error} />
					<Button variant="outline" onClick={() => void query.refetch()}>
						Retry resources
					</Button>
				</div>
			)}
			{query.data ? (
				<ResourceArchive
					key={template.id}
					template={template}
					resources={query.data}
				/>
			) : (
				<>
					<ResourceEditorHeader template={template} />
					{query.isLoading && <Loader />}
				</>
			)}
		</>
	);
};

type ResourceArchiveProps = {
	template: Template;
	resources: TemplateResources;
};

const ResourceArchive: React.FC<ResourceArchiveProps> = ({
	template,
	resources,
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
					template={template}
					resources={resources}
					initial={initial}
					revision={revision}
				/>
			) : (
				<>
					<ResourceEditorHeader template={template} />
					{query.isLoading && <Loader />}
				</>
			)}
		</>
	);
};

type ResourceEditorHeaderProps = {
	template: Template;
	children?: React.ReactNode;
};

const ResourceEditorHeader: React.FC<ResourceEditorHeaderProps> = ({
	template,
	children,
}) => {
	const getLink = useLinks();
	return (
		<>
			<Topbar className="flex items-center gap-4 pr-4 shrink-0">
				<TopbarIconButton asChild>
					<Link
						to={getLink(
							linkToTemplate(template.organization_name, template.name),
						)}
						aria-label="Back to the template"
					>
						<ChevronLeftIcon />
					</Link>
				</TopbarIconButton>
				<TopbarData>{template.display_name || template.name}</TopbarData>
				<div className="ml-auto flex gap-2">{children}</div>
			</Topbar>
			<ProvisioningScopeNavigation scope="shared" />
		</>
	);
};

type ResourceEditorSessionProps = {
	template: Template;
	resources: TemplateResources;
	initial: { archive: TarReader; files: FileTree };
	revision: TemplateResourceRevision | undefined;
};

const ResourceEditorSession: React.FC<ResourceEditorSessionProps> = ({
	template,
	resources,
	initial,
	revision,
}) => {
	const queryClient = useQueryClient();
	const [files, setFiles] = useState(initial.files);
	const [savedFiles, setSavedFiles] = useState(initial.files);
	const [built, setBuilt] = useState<
		| {
				files: FileTree;
				revision: TemplateResourceRevision;
				run: TemplateResourceRun;
		  }
		| undefined
	>(() => {
		const plan = resources.runs
			.filter((item) => item.action === "plan")
			.toSorted((a, b) => b.created_at.localeCompare(a.created_at))[0];
		return revision && plan?.revision_id === revision.id
			? { files: initial.files, revision, run: plan }
			: undefined;
	});
	const [selectedRun, setSelectedRun] = useState<
		TemplateResourceRun | undefined
	>(() => {
		const runs = resources.runs.toSorted((a, b) =>
			b.created_at.localeCompare(a.created_at),
		);
		return runs.find((item) => item.revision_id === revision?.id) ?? runs[0];
	});
	const [buildError, setBuildError] = useState<unknown>();
	const [building, setBuilding] = useState(false);
	const [params, setParams] = useSearchParams();
	const upload = useMutation(uploadFile());
	const save = useMutation(
		createTemplateResourceRevision(queryClient, template.id),
	);
	const run = useMutation(createTemplateResourceRun(queryClient, template.id));
	const dirty = files !== savedFiles;
	const liveRun =
		selectedRun &&
		(resources.runs.find((item) => item.id === selectedRun.id) ?? selectedRun);
	const plan =
		built &&
		(resources.runs.find((item) => item.id === built.run.id) ?? built.run);
	const busy =
		building ||
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
	const canPublish =
		!blocked &&
		built &&
		built.files === files &&
		plan?.job.status === "succeeded" &&
		!resources.runs.some(
			(item) =>
				item.action === "plan" &&
				item.id !== plan.id &&
				item.created_at >= plan.created_at,
		) &&
		resources.current_revision_id === built.revision.id &&
		resources.applied_revision_id !== built.revision.id;
	let buildMessage =
		"Build a successful plan before publishing. Workspace versions are not changed.";
	if (busy) {
		buildMessage =
			"Waiting for the current operation. Logs refresh automatically.";
	} else if (resources.status === "unknown") {
		buildMessage =
			"Shared resource state is unknown. Reconcile state before building or publishing.";
	} else if (dirty) {
		buildMessage =
			"Unbuilt edits. Build saves all files and plans this Terraform root.";
	} else if (
		resources.current_revision_id &&
		resources.current_revision_id === resources.applied_revision_id &&
		resources.current_revision_id === (built?.revision.id ?? revision?.id)
	) {
		buildMessage =
			"This revision is published. Build changes to publish another revision.";
	} else if (canPublish) {
		buildMessage =
			"Build succeeded. Publish applies this immutable revision to shared resources only.";
	}
	const requestedPath = params.get("path");
	const activePath =
		requestedPath &&
		existsFile(requestedPath, files) &&
		!isFolder(requestedPath, files)
			? requestedPath
			: findEntrypointFile(files);
	const build = useCallback(async () => {
		if (blocked) return;
		setBuilding(true);
		setBuilt(undefined);
		setSelectedRun(undefined);
		setBuildError(undefined);
		run.reset();
		save.reset();
		try {
			const archive = await writeTemplateArchive(initial.archive, files);
			const uploaded = await upload.mutateAsync(archive);
			const revision = await save.mutateAsync({ file_id: uploaded.hash });
			setSavedFiles(files);
			const planned = await run.mutateAsync({
				revision_id: revision.id,
				action: "plan",
			});
			setSelectedRun(planned);
			setBuilt({ files, revision, run: planned });
		} catch (error) {
			setBuildError(error);
		} finally {
			setBuilding(false);
		}
	}, [blocked, files, initial.archive, upload, save, run]);

	useEffect(() => {
		const listener = (event: KeyboardEvent) => {
			if (!(event.ctrlKey || event.metaKey)) return;
			if (event.key === "s") event.preventDefault();
			if (event.key === "Enter") {
				event.preventDefault();
				void build();
			}
		};
		document.addEventListener("keydown", listener);
		return () => document.removeEventListener("keydown", listener);
	}, [build]);
	usePrompt({
		message:
			"You have unsaved shared resource changes. Are you sure you want to leave?",
		when: ({ currentLocation, nextLocation }) =>
			(dirty || building) &&
			(currentLocation.pathname !== nextLocation.pathname ||
				new URLSearchParams(nextLocation.search).get("scope") !== "template"),
	});
	useBeforeUnload(
		useCallback(
			(event: BeforeUnloadEvent) => {
				if (dirty || building) event.preventDefault();
			},
			[dirty, building],
		),
	);

	return (
		<>
			<ResourceEditorHeader template={template}>
				<TopbarButton
					disabled={blocked}
					title="Build shared resources (Ctrl + Enter)"
					onClick={() => void build()}
				>
					<PlayIcon />
					{building ? "Building…" : "Build"}
				</TopbarButton>
				<TopbarButton
					variant="default"
					disabled={!canPublish}
					onClick={() => {
						if (canPublish && built)
							run.mutate(
								{ revision_id: built.revision.id, action: "apply" },
								{ onSuccess: setSelectedRun },
							);
					}}
				>
					Publish
				</TopbarButton>
			</ResourceEditorHeader>
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
					<section aria-label="Build output" className="p-4 space-y-3">
						<h2 className="text-sm font-semibold m-0">Output</h2>
						<p className="text-sm text-content-secondary m-0">{buildMessage}</p>
						{Boolean(buildError || run.error) && (
							<ErrorAlert error={buildError || run.error} />
						)}
						{Boolean(buildError) && save.data && (
							<p className="text-sm text-content-secondary">
								Revision {save.data.id.slice(0, 8)} was saved, but its plan
								could not be queued. Build again to retry.
							</p>
						)}
						{liveRun && (
							<>
								<p className="text-sm">
									{runActionLabels[liveRun.action] ?? liveRun.action}:{" "}
									{liveRun.job.status} · Revision{" "}
									{liveRun.revision_id.slice(0, 8)}
									{liveRun.revision_id !==
										(built?.revision.id ?? revision?.id) &&
										" (older revision, not this build)"}
								</p>
								{liveRun.job.error && <ErrorAlert error={liveRun.job.error} />}
								<SharedResourceRunLogs
									templateId={template.id}
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
