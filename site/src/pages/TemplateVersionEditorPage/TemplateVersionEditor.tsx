import IconButton from "@mui/material/IconButton";
import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	type AIBridgeModel,
	type AIModelConfig,
	aiBridgeModels,
} from "api/queries/aiBridge";
import { experiments } from "api/queries/experiments";
import type {
	ProvisionerJobLog,
	Template,
	TemplateVersion,
	TemplateVersionVariable,
	VariableValue,
	WorkspaceResource,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { Sidebar } from "components/FullPageLayout/Sidebar";
import {
	Topbar,
	TopbarAvatar,
	TopbarButton,
	TopbarData,
	TopbarDivider,
	TopbarIconButton,
} from "components/FullPageLayout/Topbar";
import { Loader } from "components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import {
	ChevronLeftIcon,
	ExternalLinkIcon,
	PlayIcon,
	PlusIcon,
	SparklesIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import {
	AlertVariant,
	ProvisionerAlert,
} from "modules/provisioners/ProvisionerAlert";
import { ProvisionerStatusAlert } from "modules/provisioners/ProvisionerStatusAlert";
import { WildcardHostnameWarning } from "modules/resources/WildcardHostnameWarning";
import { isBinaryData } from "modules/templates/TemplateFiles/isBinaryData";
import { TemplateFileTree } from "modules/templates/TemplateFiles/TemplateFileTree";
import { TemplateResourcesTable } from "modules/templates/TemplateResourcesTable/TemplateResourcesTable";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import type { PublishVersionData } from "pages/TemplateVersionEditorPage/types";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import {
	Link as RouterLink,
	useNavigate,
	unstable_usePrompt as usePrompt,
} from "react-router";
import { toast } from "sonner";
import { cn } from "utils/cn";
import {
	createFile,
	existsFile,
	type FileTree,
	getFileText,
	isFolder,
	moveFile,
	removeFile,
	updateFile,
} from "utils/filetree";
import { AIChatPanel } from "./ai/AIChatPanel";
import { getDefaultModelConfig, isCuratedModel } from "./ai/ModelConfigBar";
import type {
	BuildOutput,
	BuildResult,
	PublishRequestData,
	PublishRequestOptions,
	PublishResult,
} from "./ai/tools";
import { useTemplateAgent } from "./ai/useTemplateAgent";
import {
	CreateFileDialog,
	DeleteFileDialog,
	RenameFileDialog,
} from "./FileDialog";
import { MissingTemplateVariablesDialog } from "./MissingTemplateVariablesDialog";
import { MonacoEditor } from "./MonacoEditor";
import { ProvisionerTagsPopover } from "./ProvisionerTagsPopover";
import { PublishTemplateVersionDialog } from "./PublishTemplateVersionDialog";
import { TemplateVersionStatusBadge } from "./TemplateVersionStatusBadge";

type Tab = "logs" | "resources" | undefined; // Undefined is to hide the tab

const isLikelyChatModel = (model: AIBridgeModel): boolean => {
	if (model.provider === "anthropic") {
		return true;
	}

	const normalized = model.id.toLowerCase();
	if (
		normalized.includes("embed") ||
		normalized.includes("moderation") ||
		normalized.includes("whisper") ||
		normalized.includes("transcription") ||
		normalized.includes("tts") ||
		normalized.includes("speech") ||
		normalized.includes("image") ||
		normalized.includes("dall-e") ||
		normalized.includes("rerank")
	) {
		return false;
	}

	return (
		normalized.includes("chat") ||
		normalized.includes("instruct") ||
		normalized.startsWith("gpt-") ||
		normalized.startsWith("o1") ||
		normalized.startsWith("o3") ||
		normalized.startsWith("o4") ||
		normalized.startsWith("claude-") ||
		normalized.startsWith("gemini-")
	);
};

// Prefer a curated model, then any chat-capable model, then fall
// back to the first discovered model so deployments with custom
// model naming can still use the assistant.
const selectDefaultAIModel = (
	models: readonly AIBridgeModel[],
): AIBridgeModel | undefined => {
	const curatedModel = models.find((m) => isCuratedModel(m.id));
	if (curatedModel) {
		return curatedModel;
	}
	const likelyChatModel = models.find(isLikelyChatModel);
	if (likelyChatModel) {
		return likelyChatModel;
	}
	return models[0];
};

interface TemplateVersionEditorProps {
	template: Template;
	templateVersion: TemplateVersion;
	defaultFileTree: FileTree;
	buildLogs?: ProvisionerJobLog[];
	resources?: WorkspaceResource[];
	isBuilding: boolean;
	canPublish: boolean;
	onPreview: (files: FileTree) => Promise<void>;
	onPublish: () => void;
	onConfirmPublish: (data: PublishVersionData) => void;
	/** Publishes without navigating — used by the AI tool so the
	 *  chat session survives the publish action. */
	onPublishVersion: (data: PublishVersionData) => Promise<void>;
	onCancelPublish: () => void;
	publishingError?: unknown;
	publishedVersion?: TemplateVersion;
	onCreateWorkspace: () => void;
	isAskingPublishParameters: boolean;
	isPromptingMissingVariables: boolean;
	isPublishing: boolean;
	missingVariables?: TemplateVersionVariable[];
	onSubmitMissingVariableValues: (values: VariableValue[]) => void;
	onCancelSubmitMissingVariableValues: () => void;
	defaultTab?: Tab;
	provisionerTags: Record<string, string>;
	onUpdateProvisionerTags: (tags: Record<string, string>) => void;
	activePath: string | undefined;
	onActivePathChange: (path: string | undefined) => void;
}

export const TemplateVersionEditor: FC<TemplateVersionEditorProps> = ({
	isBuilding,
	canPublish,
	template,
	templateVersion,
	defaultFileTree,
	onPreview,
	onPublish,
	onConfirmPublish,
	onPublishVersion,
	onCancelPublish,
	isAskingPublishParameters,
	isPublishing,
	publishingError,
	publishedVersion,
	onCreateWorkspace,
	buildLogs,
	resources,
	isPromptingMissingVariables,
	missingVariables,
	onSubmitMissingVariableValues,
	onCancelSubmitMissingVariableValues,
	defaultTab,
	provisionerTags,
	onUpdateProvisionerTags,
	activePath,
	onActivePathChange,
}) => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const [selectedTab, setSelectedTab] = useState<Tab>(defaultTab);
	const [fileTree, setFileTree] = useState(defaultFileTree);
	const [createFileOpen, setCreateFileOpen] = useState(false);
	const [deleteFileOpen, setDeleteFileOpen] = useState<string>();
	const [renameFileOpen, setRenameFileOpen] = useState<string>();
	const [dirty, setDirty] = useState(false);
	const [aiPanelOpen, setAIPanelOpen] = useState(false);
	const { metadata } = useEmbeddedMetadata();
	const { data: enabledExperiments = [] } = useQuery(
		experiments(metadata.experiments),
	);
	const aiExperimentEnabled = enabledExperiments.includes("ai-template-editor");
	const { data: aiModels = [] } = useQuery({
		...aiBridgeModels(),
		enabled: aiExperimentEnabled,
	});
	const [aiModelConfig, setAIModelConfig] = useState<AIModelConfig>();
	const defaultAIModel = selectDefaultAIModel(aiModels);
	useEffect(() => {
		if (aiModelConfig === undefined) {
			if (defaultAIModel !== undefined) {
				setAIModelConfig(getDefaultModelConfig(defaultAIModel));
			}
			return;
		}

		const currentModelIsAvailable = aiModels.some(
			(model) =>
				model.id === aiModelConfig.model.id &&
				model.provider === aiModelConfig.model.provider,
		);
		if (currentModelIsAvailable) {
			return;
		}

		if (defaultAIModel !== undefined) {
			setAIModelConfig(getDefaultModelConfig(defaultAIModel));
			return;
		}

		setAIModelConfig(undefined);
	}, [aiModelConfig, aiModels, defaultAIModel]);
	const aiAvailable = aiExperimentEnabled && aiModelConfig !== undefined;

	// Use refs so that AI tool callbacks always read the latest
	// state, including eagerly applied mutations that haven't
	// flushed through a React re-render yet.
	const fileTreeRef = useRef(fileTree);
	fileTreeRef.current = fileTree;

	const dirtyRef = useRef(dirty);
	dirtyRef.current = dirty;

	const templateVersionRef = useRef(templateVersion);
	templateVersionRef.current = templateVersion;

	const buildLogsRef = useRef(buildLogs);
	buildLogsRef.current = buildLogs;

	const canPublishRef = useRef(canPublish);
	canPublishRef.current = canPublish;

	const getFileTree = useCallback(() => fileTreeRef.current, []);
	const setFileTreeAndDirty = useCallback(
		(updater: (prev: FileTree) => FileTree) => {
			const next = updater(fileTreeRef.current);
			// Update refs immediately so resumed agent loops
			// in this tick observe the latest editor state.
			fileTreeRef.current = next;
			dirtyRef.current = true;
			setFileTree(next);
			setDirty(true);
		},
		[],
	);

	// Wrap onActivePathChange so the AI agent can navigate to a
	// file only if it actually exists and is not a folder.
	const navigateToExistingFile = useCallback(
		(path: string) => {
			if (path.length === 0) {
				return;
			}
			const tree = fileTreeRef.current;
			if (!existsFile(path, tree) || isFolder(path, tree)) {
				return;
			}
			onActivePathChange(path);
		},
		[onActivePathChange],
	);

	// Monotonic ID for build waiters. This prevents stale build completions
	// from resolving a promise registered for a newer build.
	const buildIdRef = useRef(0);
	// Ref: resolver for the in-flight build promise.
	const buildCompleteResolverRef = useRef<{
		id: number;
		resolve: (result: BuildResult) => void;
	} | null>(null);

	const triggerBuild = useCallback(async () => {
		await onPreview(getFileTree());
	}, [onPreview, getFileTree]);

	const waitForBuildComplete = useCallback((): Promise<BuildResult> => {
		return new Promise<BuildResult>((resolve) => {
			const id = buildIdRef.current + 1;
			buildIdRef.current = id;
			buildCompleteResolverRef.current = { id, resolve };
		});
	}, []);

	const getBuildOutput = useCallback((): BuildOutput | undefined => {
		const currentTemplateVersion = templateVersionRef.current;
		const status = currentTemplateVersion.job.status;
		if (!status) {
			return undefined;
		}
		const logText = (buildLogsRef.current ?? [])
			.map((l) => `[${l.log_level}] ${l.stage}: ${l.output}`)
			.join("\n");
		return { status, error: currentTemplateVersion.job.error, logs: logText };
	}, []);

	const handlePublish = useCallback(
		async (
			data: PublishRequestData,
			options?: PublishRequestOptions,
		): Promise<PublishResult> => {
			const currentTemplateVersion = templateVersionRef.current;
			if (dirtyRef.current && !options?.skipDirtyCheck) {
				return {
					success: false,
					error: "There are unsaved changes. Build the template first.",
				};
			}
			if (currentTemplateVersion.job.status !== "succeeded") {
				return {
					success: false,
					error: "Cannot publish — the build must succeed first.",
				};
			}
			if (!canPublishRef.current) {
				return {
					success: false,
					error: "This version is already the active version.",
				};
			}
			try {
				// Use onPublishVersion (no navigation) instead of
				// onConfirmPublish so the chat session survives.
				await onPublishVersion({
					name: data.name ?? currentTemplateVersion.name,
					message: data.message ?? "",
					isActiveVersion: data.isActiveVersion ?? true,
				});
				return {
					success: true,
					versionName: data.name ?? currentTemplateVersion.name,
				};
			} catch (err) {
				const msg = err instanceof Error ? err.message : "Failed to publish";
				return { success: false, error: msg };
			}
		},
		[onPublishVersion],
	);

	// The agent hook lives here (not inside AIChatPanel) so that
	// chat history survives the panel being toggled open/closed.
	// The panel merely presents the state this hook manages.
	const templateAgent = useTemplateAgent({
		getFileTree,
		setFileTree: setFileTreeAndDirty,
		modelConfig: aiModelConfig ?? { model: { id: "", provider: "openai" } },
		onFileEdited: navigateToExistingFile,
		onFileDeleted: (path) => {
			if (activePath === path) {
				onActivePathChange(undefined);
			}
		},
		onBuildRequested: triggerBuild,
		waitForBuildComplete,
		getBuildOutput,
		onPublishRequested: handlePublish,
	});

	const { resetBuildState } = templateAgent;

	// Manual editor/file-tree edits set dirty=true outside the AI tool flow.
	// Invalidate build state so publish cannot skip the dirty check.
	useEffect(() => {
		if (!dirty) {
			return;
		}
		resetBuildState();
	}, [dirty, resetBuildState]);

	// Resolve the build promise when job status becomes terminal.
	useEffect(() => {
		const pending = buildCompleteResolverRef.current;
		if (!pending) {
			return;
		}
		// Ignore terminal status updates that belong to an older build waiter.
		if (pending.id !== buildIdRef.current) {
			return;
		}
		const status = templateVersion.job.status;
		if (
			status === "succeeded" ||
			status === "failed" ||
			status === "canceled" ||
			status === "unknown"
		) {
			const logText = (buildLogs ?? [])
				.map((l) => `[${l.log_level}] ${l.stage}: ${l.output}`)
				.join("\n");
			pending.resolve({
				status: status === "unknown" ? "failed" : status,
				error:
					status === "unknown"
						? (templateVersion.job.error ??
							"Build ended with an unknown status.")
						: templateVersion.job.error,
				logs: logText,
			});
			buildCompleteResolverRef.current = null;
		}
	}, [templateVersion.job.status, templateVersion.job.error, buildLogs]);

	// Abort any active stream when the page-level component is
	// unmounted so we don't leave orphaned network requests.
	// biome-ignore lint/correctness/useExhaustiveDependencies: cleanup runs only on unmount
	useEffect(() => {
		return () => {
			templateAgent.stop();
			const pending = buildCompleteResolverRef.current;
			if (pending && pending.id === buildIdRef.current) {
				pending.resolve({
					status: "canceled",
					error: "Agent stopped.",
					logs: "",
				});
				buildCompleteResolverRef.current = null;
			}
		};
		// eslint-disable-next-line react-hooks/exhaustive-deps -- run only on unmount
	}, []);

	const matchingProvisioners = templateVersion.matched_provisioners?.count;
	const availableProvisioners = templateVersion.matched_provisioners?.available;

	const triggerPreview = useCallback(async () => {
		try {
			await onPreview(fileTree);
			setSelectedTab("logs");
		} catch (error) {
			toast.error(getErrorMessage(error, "Error on previewing the template."), {
				description: getErrorDetail(error),
			});
		}
	}, [fileTree, onPreview]);

	// Stop ctrl+s from saving files and make ctrl+enter trigger a preview.
	useEffect(() => {
		const keyListener = async (event: KeyboardEvent) => {
			if (!(navigator.platform.match("Mac") ? event.metaKey : event.ctrlKey)) {
				return;
			}
			switch (event.key) {
				case "s":
					// Prevent opening the save dialog!
					event.preventDefault();
					break;
				case "Enter":
					event.preventDefault();
					await triggerPreview();
					break;
			}
		};
		document.addEventListener("keydown", keyListener);
		return () => {
			document.removeEventListener("keydown", keyListener);
		};
	}, [triggerPreview]);

	const canBuild = !isBuilding;
	const templateLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);

	// Automatically switch to the template preview tab when the build succeeds.
	const previousVersion = useRef<TemplateVersion>(undefined);
	useEffect(() => {
		if (!previousVersion.current) {
			previousVersion.current = templateVersion;
			return;
		}

		if (
			["running", "pending"].includes(previousVersion.current.job.status) &&
			templateVersion.job.status === "succeeded"
		) {
			setDirty(false);
			toast.success(
				`Template version "${previousVersion.current.name}" built successfully.`,
				{
					action: {
						label: "View template",
						onClick: () => navigate(templateLink),
					},
				},
			);
		}
		previousVersion.current = templateVersion;
	}, [templateVersion, navigate, templateLink]);

	const editorValue = activePath ? getFileText(activePath, fileTree) : "";
	const isEditorValueBinary =
		typeof editorValue === "string" ? isBinaryData(editorValue) : false;

	useLeaveSiteWarning(dirty);

	const gotBuildLogs = buildLogs && buildLogs.length > 0;

	return (
		<>
			<div className="h-full flex flex-col">
				<Topbar className="grid grid-cols-[1fr_2fr_1fr]" data-testid="topbar">
					<div>
						<Tooltip>
							<TooltipTrigger asChild>
								<TopbarIconButton component={RouterLink} to={templateLink}>
									<ChevronLeftIcon className="size-icon-sm" />
								</TopbarIconButton>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								Back to the template
							</TooltipContent>
						</Tooltip>
					</div>

					<TopbarData>
						<TopbarAvatar
							src={template.icon}
							fallback={template.display_name || template.name}
						/>
						<RouterLink
							to={templateLink}
							className="text-content-primary no-underline hover:underline"
						>
							{template.display_name || template.name}
						</RouterLink>
						<TopbarDivider />
						<span className="text-content-secondary">
							{templateVersion.name}
						</span>
					</TopbarData>

					<div className="flex items-center justify-end gap-2 pr-4">
						{aiAvailable && (
							<TopbarIconButton
								title="AI Assistant"
								onClick={() => {
									setAIPanelOpen((v) => !v);
								}}
								className={aiPanelOpen ? "text-content-link" : undefined}
							>
								<SparklesIcon className="size-icon-sm" />
							</TopbarIconButton>
						)}
						<span className="mr-2">
							<Button asChild size="sm" variant="outline">
								<a
									href="https://registry.coder.com"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center"
								>
									Browse the Coder Registry
									<ExternalLinkIcon className="size-icon-sm ml-1" />
								</a>
							</Button>
						</span>

						<TemplateVersionStatusBadge version={templateVersion} />

						<div className="flex gap-1 items-center">
							<TopbarButton
								title="Build template (Ctrl + Enter)"
								disabled={!canBuild}
								onClick={async () => {
									await triggerPreview();
								}}
							>
								<PlayIcon />
								Build
							</TopbarButton>
							<ProvisionerTagsPopover
								tags={provisionerTags}
								onTagsChange={onUpdateProvisionerTags}
							/>
						</div>

						<TopbarButton
							variant="default"
							disabled={dirty || !canPublish}
							onClick={onPublish}
						>
							Publish
						</TopbarButton>
					</div>
				</Topbar>

				<div className="flex flex-1 basis-0 overflow-hidden relative">
					{publishedVersion && (
						<div
							// We need this to reset the dismissable state of the component
							// when the published version changes
							key={publishedVersion.id}
							className="absolute w-full flex justify-center p-3 z-10"
						>
							<Alert
								severity="success"
								prominent
								dismissible
								actions={
									<Button
										variant="subtle"
										size="sm"
										onClick={onCreateWorkspace}
									>
										Create a workspace
									</Button>
								}
							>
								Successfully published {publishedVersion.name}!
							</Alert>
						</div>
					)}

					<Sidebar>
						<div className="h-[42px] py-0 pr-2 pl-4 flex items-center">
							<span className="text-content-primary text-[13px]">Files</span>

							<div className="ml-auto [&_svg]:fill-content-primary">
								<Tooltip>
									<TooltipTrigger asChild>
										<IconButton
											aria-label="Create File"
											onClick={(event) => {
												setCreateFileOpen(true);
												event.currentTarget.blur();
											}}
										>
											<PlusIcon className="size-icon-xs" />
										</IconButton>
									</TooltipTrigger>
									<TooltipContent>Create File</TooltipContent>
								</Tooltip>
							</div>
							<CreateFileDialog
								fileTree={fileTree}
								open={createFileOpen}
								onClose={() => {
									setCreateFileOpen(false);
								}}
								checkExists={(path) => existsFile(path, fileTree)}
								onConfirm={(path) => {
									setFileTree((fileTree) => createFile(path, fileTree, ""));
									onActivePathChange(path);
									setCreateFileOpen(false);
									setDirty(true);
								}}
							/>
							<DeleteFileDialog
								onConfirm={() => {
									if (!deleteFileOpen) {
										throw new Error("delete file must be set");
									}
									setFileTree((fileTree) =>
										removeFile(deleteFileOpen, fileTree),
									);
									setDeleteFileOpen(undefined);
									if (activePath === deleteFileOpen) {
										onActivePathChange(undefined);
									}
									setDirty(true);
								}}
								open={Boolean(deleteFileOpen)}
								onClose={() => setDeleteFileOpen(undefined)}
								filename={deleteFileOpen || ""}
							/>
							<RenameFileDialog
								fileTree={fileTree}
								open={Boolean(renameFileOpen)}
								onClose={() => {
									setRenameFileOpen(undefined);
								}}
								filename={renameFileOpen || ""}
								checkExists={(path) => existsFile(path, fileTree)}
								onConfirm={(newPath) => {
									if (!renameFileOpen) {
										return;
									}
									setFileTree((fileTree) =>
										moveFile(renameFileOpen, newPath, fileTree),
									);
									onActivePathChange(newPath);
									setRenameFileOpen(undefined);
									setDirty(true);
								}}
							/>
						</div>
						<TemplateFileTree
							fileTree={fileTree}
							onDelete={(file) => setDeleteFileOpen(file)}
							onSelect={(filePath) => {
								if (!isFolder(filePath, fileTree)) {
									onActivePathChange(filePath);
								}
							}}
							onRename={(file) => setRenameFileOpen(file)}
							activePath={activePath}
						/>
					</Sidebar>

					<PanelGroup
						direction="horizontal"
						autoSaveId="template-editor-ai"
						className="w-full min-h-full overflow-hidden"
					>
						<Panel
							id="template-editor-main"
							order={1}
							minSize={40}
							className="[&>*]:h-full"
						>
							<div className="flex flex-col w-full min-h-full overflow-hidden">
								<div className="flex-1 overflow-y-auto" data-chromatic="ignore">
									{activePath ? (
										isEditorValueBinary ? (
											<div
												role="alert"
												className="w-full h-full flex items-center justify-center p-10"
											>
												<div className="flex flex-col items-center max-w-[420px] text-center">
													<TriangleAlertIcon className="text-content-warning size-icon-lg" />
													<p className="m-0 p-0 mt-6">
														The file is not displayed in the text editor because
														it is either binary or uses an unsupported text
														encoding.
													</p>
												</div>
											</div>
										) : (
											<MonacoEditor
												value={editorValue}
												path={activePath}
												onChange={(value) => {
													if (!activePath) {
														return;
													}
													setFileTree((fileTree) =>
														updateFile(activePath, value, fileTree),
													);
													setDirty(true);
												}}
											/>
										)
									) : (
										<div>No file opened</div>
									)}
								</div>

								<div className="border-0 border-t border-solid border-border overflow-hidden flex flex-col">
									<div
										className={cn(
											"flex items-center",
											selectedTab &&
												"border-0 border-b border-solid border-border",
										)}
									>
										<div className="flex">
											<button
												type="button"
												disabled={!buildLogs}
												className={tabClassName(selectedTab === "logs")}
												onClick={() => {
													setSelectedTab("logs");
												}}
											>
												Output
											</button>

											<button
												type="button"
												disabled={!canPublish}
												className={tabClassName(selectedTab === "resources")}
												onClick={() => {
													setSelectedTab("resources");
												}}
											>
												Resources
											</button>
										</div>

										{selectedTab === "logs" && gotBuildLogs && (
											<a
												href={`/api/v2/templateversions/${templateVersion.id}/logs?format=text`}
												target="_blank"
												rel="noopener noreferrer"
												className="flex items-center gap-1 px-3 text-xs text-content-secondary hover:text-content-primary"
											>
												View raw logs
												<ExternalLinkIcon className="size-3" />
											</a>
										)}

										{selectedTab && (
											<IconButton
												onClick={() => {
													setSelectedTab(undefined);
												}}
												className={cn(
													"w-9 h-9 rounded-none",
													(selectedTab !== "logs" || !gotBuildLogs) &&
														"ml-auto",
												)}
											>
												<XIcon className="size-icon-xs" />
											</IconButton>
										)}
									</div>

									{selectedTab === "logs" && (
										<div className="flex flex-col h-[280px] overflow-y-auto">
											{templateVersion.job.error ? (
												<div>
													<ProvisionerAlert
														title="Error during the build"
														detail={templateVersion.job.error}
														severity="error"
														tags={templateVersion.job.tags}
														variant={AlertVariant.Inline}
													/>
												</div>
											) : (
												!gotBuildLogs && (
													<>
														<ProvisionerStatusAlert
															matchingProvisioners={matchingProvisioners}
															availableProvisioners={availableProvisioners}
															tags={templateVersion.job.tags}
															variant={AlertVariant.Inline}
														/>
														<Loader className="h-full" />
													</>
												)
											)}

											{gotBuildLogs && (
												<WorkspaceBuildLogs
													className={cn(
														"rounded-none border-0",
														"[&_.logs-header]:border-0 [&_.logs-header]:px-4 [&_.logs-header]:py-2 [&_.logs-header]:font-mono",
														"[&_.logs-header:first-of-type]:pt-4 [&_.logs-header:last-child]:pb-4",
														"[&_.logs-line]:pl-4",
														"[&_.logs-container]:!border-0",
													)}
													hideTimestamps
													logs={buildLogs}
												/>
											)}

											{resources && (
												<WildcardHostnameWarning resources={resources} />
											)}
										</div>
									)}

									{selectedTab === "resources" && (
										<div
											className={cn(
												"h-[280px] overflow-y-auto",
												"[&_.resource-card]:border-l-0 [&_.resource-card]:border-r-0",
												"[&_.resource-card:first-of-type]:border-t-0 [&_.resource-card:last-child]:border-b-0",
											)}
										>
											{resources && (
												<TemplateResourcesTable
													resources={resources.filter(
														(r) => r.workspace_transition === "start",
													)}
												/>
											)}
										</div>
									)}
								</div>
							</div>
						</Panel>
						{aiPanelOpen && aiModelConfig && (
							<>
								<PanelResizeHandle>
									<div className="h-full w-1 bg-border transition-colors hover:bg-content-link" />
								</PanelResizeHandle>
								<Panel
									id="template-editor-ai"
									order={2}
									defaultSize={30}
									minSize={20}
								>
									<AIChatPanel
										agent={templateAgent}
										getFileTree={getFileTree}
										modelConfig={aiModelConfig}
										availableModels={aiModels}
										onModelConfigChange={setAIModelConfig}
										onNavigateToFile={navigateToExistingFile}
										onClose={() => {
											templateAgent.stop();
											setAIPanelOpen(false);
										}}
									/>
								</Panel>
							</>
						)}
					</PanelGroup>
				</div>
			</div>

			<PublishTemplateVersionDialog
				key={templateVersion.name}
				publishingError={publishingError}
				open={isAskingPublishParameters || isPublishing}
				onClose={onCancelPublish}
				onConfirm={onConfirmPublish}
				isPublishing={isPublishing}
				defaultName={templateVersion.name}
			/>

			<MissingTemplateVariablesDialog
				open={isPromptingMissingVariables}
				onClose={onCancelSubmitMissingVariableValues}
				onSubmit={onSubmitMissingVariableValues}
				missingVariables={missingVariables}
			/>
		</>
	);
};

const useLeaveSiteWarning = (enabled: boolean) => {
	const MESSAGE =
		"You have unpublished changes. Are you sure you want to leave?";

	// This works for regular browser actions like close tab and back button
	useEffect(() => {
		const onBeforeUnload = (e: BeforeUnloadEvent) => {
			if (enabled) {
				e.preventDefault();
				return MESSAGE;
			}
		};

		window.addEventListener("beforeunload", onBeforeUnload);

		return () => {
			window.removeEventListener("beforeunload", onBeforeUnload);
		};
	}, [enabled]);

	// This is used for react router navigation that is not triggered by the
	// browser
	usePrompt({
		message: MESSAGE,
		when: ({ nextLocation }) => {
			// We need to check the path because we change the URL when new template
			// version is created during builds
			return enabled && !nextLocation.pathname.endsWith("/edit");
		},
	});
};

const tabClassName = (isActive: boolean) =>
	cn(
		"p-3 text-[10px] uppercase tracking-[0.5px] font-medium bg-transparent [font-family:inherit] border-0",
		"text-content-secondary transition-all duration-150",
		"flex gap-2 items-center justify-center relative",
		"[&_svg]:max-w-3 [&_svg]:max-h-3",
		"enabled:cursor-pointer",
		"hover:enabled:text-content-primary",
		"disabled:text-content-disabled",
		isActive && [
			"text-content-primary",
			"after:content-[''] after:block after:w-full after:h-px after:bg-surface-invert-primary after:absolute after:-bottom-px",
		],
	);
