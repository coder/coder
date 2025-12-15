import { useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import { getErrorDetail, getErrorMessage } from "api/errors";
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
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	ChevronLeftIcon,
	ExternalLinkIcon,
	PlayIcon,
	PlusIcon,
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
import {
	Link as RouterLink,
	unstable_usePrompt as usePrompt,
} from "react-router";
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
	const getLink = useLinks();
	const theme = useTheme();
	const [selectedTab, setSelectedTab] = useState<Tab>(defaultTab);
	const [fileTree, setFileTree] = useState(defaultFileTree);
	const [createFileOpen, setCreateFileOpen] = useState(false);
	const [deleteFileOpen, setDeleteFileOpen] = useState<string>();
	const [renameFileOpen, setRenameFileOpen] = useState<string>();
	const [dirty, setDirty] = useState(false);
	const matchingProvisioners = templateVersion.matched_provisioners?.count;
	const availableProvisioners = templateVersion.matched_provisioners?.available;

	const triggerPreview = useCallback(async () => {
		try {
			await onPreview(fileTree);
			setSelectedTab("logs");
		} catch (error) {
			displayError(
				getErrorMessage(error, "Error on previewing the template"),
				getErrorDetail(error),
			);
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
			displaySuccess(
				`Template version "${previousVersion.current.name}" built successfully.`,
			);
		}
		previousVersion.current = templateVersion;
	}, [templateVersion]);

	const editorValue = activePath ? getFileText(activePath, fileTree) : "";
	const isEditorValueBinary =
		typeof editorValue === "string" ? isBinaryData(editorValue) : false;

	useLeaveSiteWarning(dirty);

	const canBuild = !isBuilding;
	const templateLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);

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

				<div className="flex flex-1 flex-basis-0 overflow-hidden relative">
					{publishedVersion && (
						<div
							// We need this to reset the dismissable state of the component
							// when the published version changes
							key={publishedVersion.id}
							className="absolute w-full flex justify-center p-3 z-10"
						>
							<Alert
								severity="success"
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
						<div className="h-[42px] pr-2 pl-4 flex items-center">
							<span className="text-content-primary text-[13px]">Files</span>

							<div className="ml-auto [&_svg]:text-content-primary">
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

					<div className="flex flex-col w-full min-h-full overflow-hidden">
						<div className="flex-1 overflow-y-auto" data-chromatic="ignore">
							{activePath ? (
								isEditorValueBinary ? (
									<div
										role="alert"
										className="w-full h-full flex items-center justify-center p-10"
									>
										<div className="flex flex-col items-center max-w-105 text-center">
											<TriangleAlertIcon
												css={{
													color: theme.roles.warning.fill.outline,
												}}
												className="size-icon-lg"
											/>
											<p className="m-0 p-0 mt-6">
												The file is not displayed in the text editor because it
												is either binary or uses an unsupported text encoding.
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

						<div className="overflow-hidden flex flex-col border-0 border-t border-solid border-border">
							<div
								className={cn(
									"flex items-center",
									selectedTab && "border-0 border-b border-solid border-border",
								)}
							>
								<div
									className={cn(
										"flex",
										"[&_.MuiTab-root]:p-0 [&_.MuiTab-root]:text-[14px]",
										"[&_.MuiTab-root]:[text-transform:none]",
										"[&_.MuiTab-root]:tracking-[unset]",
									)}
								>
									<button
										type="button"
										disabled={!buildLogs}
										className={cn(
											classNames.tab,
											selectedTab === "logs" && "active",
										)}
										onClick={() => {
											setSelectedTab("logs");
										}}
									>
										Output
									</button>

									<button
										type="button"
										disabled={!canPublish}
										className={cn(
											classNames.tab,
											selectedTab === "resources" && "active",
										)}
										onClick={() => {
											setSelectedTab("resources");
										}}
									>
										Resources
									</button>
								</div>

								{selectedTab && (
									<IconButton
										onClick={() => {
											setSelectedTab(undefined);
										}}
										className="ml-auto size-9 rounded-none"
									>
										<XIcon className="size-icon-xs" />
									</IconButton>
								)}
							</div>

							{selectedTab === "logs" && (
								<div className={cn(classNames.logs, classNames.tabContent)}>
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
											className={classNames.buildLogs}
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
									className={cn(classNames.resources, classNames.tabContent)}
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
				</div>
			</div>

			<div className={"duratio"}></div>

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

const classNames = {
	tab: cn(
		"p-3 text-[10px] uppercase tracking-[0.5px]",
		"text-regular bg-transparent font-[inherit] border-0",
		"text-content-secondary transition-all duration-150",
		"flex gap-2 items-center justify-center relative",
		"[&:not(:disabled)]:cursor-pointer",
		"[&_svg]:max-w-3 [&_svg]:max-h-3",
		"[&.active]:text-content-primary [&.active]:after:content-[''] [&.active]:after:block",
		"[&.active]:after:bg-sky-500 [&.active]:after:bottom-[-1px] [&.active]:after:absolute",
		"[&.active]:after:w-full [&.active]:after:h-[1px]",
		"[&:not(:disabled):hover]:text-content-primary",
		"disabled:text-content-quaternary",
	),
	tabBar: cn(
		"py-2 px-4 sticky top-0 bg-content-primary",
		"border-0 border-b border-solid border-border",
		"text-content-primary text-xs uppercase",
		"[&.top]:border-0 [&.top]:border-t [&.top]:border-solid [&.top]:border-border",
	),
	tabContent: "h-70 overflow-y-auto",
	logs: "flex flex-col h-full",
	buildLogs: cn(
		"border-0 rounded-[0px]",
		// Hack to update logs header and lines
		"[&_.logs-header]:border-0 [&_.logs-header]:py-2",
		"[&_.logs-header]:px-4 [&_.logs-header]:font-mono",
		// Hack to update logs header and lines
		"[&_.logs-header]:first-of-type:pt-4 [&_.logs-header]:last-child:pb-4",
		"[&_.logs-line]:pl-4 [&_.logs-container]:!border-0",
	),
	resources: cn(
		// Hack to access customize resource-card from here
		"[&_.resource-card]:border-l-0 [&_.resource-card]:border-r-0",
		"[&_.resource-card:first-of-type]:rounded-t-0",
		"[&_.resource-card:last-child]:border-b-0",
	),
};
