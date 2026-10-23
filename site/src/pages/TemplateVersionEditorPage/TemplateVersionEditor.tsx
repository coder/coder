import { cn } from "cn";
import {
	ChevronLeftIcon,
	ExternalLinkIcon,
	PlayIcon,
	XIcon,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import {
	Link as RouterLink,
	useNavigate,
	unstable_usePrompt as usePrompt,
} from "react-router";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type {
	ProvisionerJobLog,
	Template,
	TemplateVersion,
	TemplateVersionVariable,
	VariableValue,
	WorkspaceResource,
} from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import {
	Topbar,
	TopbarAvatar,
	TopbarButton,
	TopbarData,
	TopbarDivider,
	TopbarIconButton,
} from "#/components/FullPageLayout/Topbar";
import { Loader } from "#/components/Loader/Loader";
import { toast } from "#/components/Toaster/toast";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import {
	AlertVariant,
	ProvisionerAlert,
} from "#/modules/provisioners/ProvisionerAlert";
import { ProvisionerStatusAlert } from "#/modules/provisioners/ProvisionerStatusAlert";
import { WildcardHostnameWarning } from "#/modules/resources/WildcardHostnameWarning";
import { TemplateResourcesTable } from "#/modules/templates/TemplateResourcesTable/TemplateResourcesTable";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import type { PublishVersionData } from "#/pages/TemplateVersionEditorPage/types";
import type { FileTree } from "#/utils/filetree";
import { MissingTemplateVariablesDialog } from "./MissingTemplateVariablesDialog";
import { ProvisionerTagsPopover } from "./ProvisionerTagsPopover";
import { PublishTemplateVersionDialog } from "./PublishTemplateVersionDialog";
import { TemplateFileEditor } from "./TemplateFileEditor";
import { TemplateVersionStatusBadge } from "./TemplateVersionStatusBadge";

type Tab = "logs" | "resources" | undefined; // Undefined is to hide the tab

type TemplateVersionEditorProps = {
	scopeNavigation?: React.ReactNode;
	template: Template;
	templateVersion: TemplateVersion;
	fileTree: FileTree;
	onFileTreeChange: (updater: (fileTree: FileTree) => FileTree) => void;
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
	createWorkspaceUrl: string | undefined;
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
};

export const TemplateVersionEditor: React.FC<TemplateVersionEditorProps> = ({
	scopeNavigation,
	isBuilding,
	canPublish,
	template,
	templateVersion,
	fileTree,
	onFileTreeChange,
	onPreview,
	onPublish,
	onConfirmPublish,
	onCancelPublish,
	isAskingPublishParameters,
	isPublishing,
	publishingError,
	publishedVersion,
	createWorkspaceUrl,
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
	const [dirty, setDirty] = useState(false);
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

	useLeaveSiteWarning(dirty);

	const gotBuildLogs = buildLogs && buildLogs.length > 0;

	return (
		<>
			<div className="h-full flex flex-col">
				<Topbar className="grid grid-cols-[1fr_2fr_1fr]" data-testid="topbar">
					<div>
						<Tooltip>
							<TooltipTrigger asChild>
								<TopbarIconButton asChild>
									<RouterLink
										to={templateLink}
										aria-label="Back to the template"
									>
										<ChevronLeftIcon />
									</RouterLink>
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
				{scopeNavigation}

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
									<Button asChild size="sm">
										<RouterLink to={createWorkspaceUrl ?? ""}>
											Create a workspace
										</RouterLink>
									</Button>
								}
							>
								<AlertTitle>
									Successfully published {publishedVersion.name}!
								</AlertTitle>
							</Alert>
						</div>
					)}

					<TemplateFileEditor
						fileTree={fileTree}
						activePath={activePath}
						onActivePathChange={onActivePathChange}
						onFileTreeChange={(updater) => {
							onFileTreeChange(updater);
							setDirty(true);
						}}
					>
						<div className="border-0 border-t border-solid border-border overflow-hidden flex flex-col">
							<div
								className={cn(
									"flex items-center",
									selectedTab && "border-0 border-b border-solid border-border",
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
									<Button
										size="icon"
										variant="subtle"
										onClick={() => {
											setSelectedTab(undefined);
										}}
										className={cn(
											(selectedTab !== "logs" || !gotBuildLogs) && "ml-auto",
										)}
									>
										<XIcon />
									</Button>
								)}
							</div>

							{selectedTab === "logs" && (
								<div className="flex flex-col h-[280px] overflow-y-auto">
									{templateVersion.job.error ? (
										<div className="sticky top-0 z-10">
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
												"[&_.logs-container]:border-0!",
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
					</TemplateFileEditor>
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
			return (
				enabled &&
				(!nextLocation.pathname.endsWith("/edit") ||
					new URLSearchParams(nextLocation.search).get("scope") === "template")
			);
		},
	});
};

const tabClassName = (isActive: boolean) =>
	cn(
		"p-3 text-[10px] uppercase tracking-[0.5px] font-medium bg-transparent font-[inherit] border-0",
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
