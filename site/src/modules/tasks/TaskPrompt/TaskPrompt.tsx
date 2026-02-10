import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { templateVersionPresets } from "api/queries/templates";
import type {
	Preset,
	Task,
	Template,
	TemplateVersionExternalAuth,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Kbd, KbdGroup } from "components/Kbd/Kbd";
import { Link } from "components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectValue,
} from "components/Select/Select";
import { Skeleton } from "components/Skeleton/Skeleton";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks/useAuthenticated";
import { useExternalAuth } from "hooks/useExternalAuth";
import { ArrowUpIcon, InfoIcon, RedoIcon, RotateCcwIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize, {
	type TextareaAutosizeProps,
} from "react-textarea-autosize";
import { docs } from "utils/docs";
import { getOSKey } from "utils/platform";
import { PromptSelectTrigger } from "./PromptSelectTrigger";
import { TemplateVersionSelect } from "./TemplateVersionSelect";

type TaskPromptProps = {
	templates: Template[] | undefined;
	error: unknown;
	onRetry: () => void;
};

export const TaskPrompt: FC<TaskPromptProps> = ({
	templates,
	error,
	onRetry,
}) => {
	if (error) {
		return <TaskPromptLoadingError error={error} onRetry={onRetry} />;
	}
	if (templates === undefined) {
		return <TaskPromptSkeleton />;
	}
	if (templates.length === 0) {
		return <TaskPromptEmpty />;
	}
	return (
		<CreateTaskForm
			templates={templates}
			onSuccess={() => {
				displaySuccess("Task created successfully");
			}}
		/>
	);
};

const TaskPromptLoadingError: FC<{
	error: unknown;
	onRetry: () => void;
}> = ({ error, onRetry }) => {
	return (
		<div className="border border-solid rounded-lg w-full min-h-80 flex items-center justify-center">
			<div className="flex flex-col items-center">
				<h3 className="m-0 font-medium text-content-primary text-base">
					{getErrorMessage(error, "Error loading Task templates")}
				</h3>
				<span className="text-content-secondary text-sm">
					{getErrorDetail(error) ?? "Please try again"}
				</span>
				<Button size="sm" onClick={onRetry} className="mt-4">
					<RotateCcwIcon />
					Try again
				</Button>
			</div>
		</div>
	);
};

const TaskPromptSkeleton: FC = () => {
	return (
		<div className="border border-border border-solid rounded-3xl p-3 bg-surface-secondary">
			{/* Textarea skeleton */}
			<PromptTextarea disabled />

			{/* Bottom controls skeleton */}
			<div className="flex items-center justify-between pt-2">
				<Skeleton className="w-[208px] h-8 rounded-full" />
				<Skeleton className="size-8 rounded-full" />
			</div>
		</div>
	);
};

const TaskPromptEmpty: FC = () => {
	return (
		<div className="rounded-lg border border-solid border-border w-full min-h-80 p-4 flex items-center justify-center">
			<div className="flex flex-col items-center">
				<h3 className="m-0 font-medium text-content-primary text-base">
					No Task templates found
				</h3>
				<span className="text-content-secondary text-sm">
					<Link href={docs("/ai-coder/tasks")} target="_blank" rel="noreferrer">
						Learn about Tasks
					</Link>{" "}
					to get started.
				</span>
			</div>
		</div>
	);
};

type CreateTaskMutationFnProps = {
	prompt: string;
};

type CreateTaskFormProps = {
	templates: Template[];
	onSuccess: (task: Task) => void;
};

const CreateTaskForm: FC<CreateTaskFormProps> = ({ templates, onSuccess }) => {
	const { user, permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const [prompt, setPrompt] = useState("");

	// Template
	const [selectedTemplateId, setSelectedTemplateId] = useState<string>(
		templates[0].id,
	);
	const selectedTemplate = templates.find(
		(t) => t.id === selectedTemplateId,
	) as Template;

	// Template versions
	const [selectedVersionId, setSelectedVersionId] = useState(
		selectedTemplate.active_version_id,
	);
	useEffect(() => {
		setSelectedVersionId(selectedTemplate.active_version_id);
	}, [selectedTemplate]);

	// Presets
	const { data: presets, isLoading: isLoadingPresets } = useQuery(
		templateVersionPresets(selectedVersionId),
	);
	const [selectedPresetId, setSelectedPresetId] = useState<string>();
	useEffect(() => {
		const defaultPreset = presets?.find((p) => p.Default);
		setSelectedPresetId(defaultPreset?.ID ?? presets?.[0]?.ID);
	}, [presets]);
	// External Auth
	const {
		externalAuth,
		externalAuthError,
		isPollingExternalAuth,
		isLoadingExternalAuth,
	} = useExternalAuth(selectedVersionId);
	const missedExternalAuth = externalAuth?.filter(
		(auth) => !auth.optional && !auth.authenticated,
	);
	const isMissingExternalAuth = missedExternalAuth
		? missedExternalAuth.length > 0
		: true;

	const createTaskMutation = useMutation({
		mutationFn: async ({ prompt }: CreateTaskMutationFnProps) => {
			// Users with updateTemplates permission can select the version to use.
			if (permissions.updateTemplates) {
				return API.createTask(user.id, {
					input: prompt,
					template_version_id: selectedVersionId,
					template_version_preset_id: selectedPresetId,
				});
			}

			// For regular users we want to enforce task creation to always use the latest
			// active template version, to avoid issues when the active version changes
			// between template load and user action.
			return createTaskWithLatestTemplateVersion(
				prompt,
				user.id,
				selectedTemplate.id,
				selectedPresetId,
			);
		},
		onSuccess: async (task) => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
			onSuccess(task);
		},
	});

	const onSubmit = async (e: React.SyntheticEvent) => {
		e.preventDefault();

		try {
			await createTaskMutation.mutateAsync({
				prompt,
			});
			setPrompt("");
		} catch (error) {
			const message = getErrorMessage(error, "Error creating task");
			const detail = getErrorDetail(error) ?? "Please try again";
			displayError(message, detail);
		}
	};

	const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
		// Submit form on Cmd+Enter (Mac) or Ctrl+Enter (Windows/Linux)
		if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
			onSubmit(e);
		}
	};

	return (
		<form
			onSubmit={onSubmit}
			aria-label="Create AI task"
			className="flex flex-col gap-4"
		>
			{externalAuthError && <ErrorAlert error={externalAuthError} />}

			<fieldset
				className="border border-border border-solid rounded-3xl p-3 bg-surface-secondary min-w-0"
				disabled={createTaskMutation.isPending}
			>
				<label htmlFor="prompt" className="sr-only">
					Prompt
				</label>
				<PromptTextarea
					required
					value={prompt}
					onChange={(e) => setPrompt(e.target.value)}
					isSubmitting={createTaskMutation.isPending}
					onKeyDown={handleKeyDown}
				/>
				<div className="flex flex-col md:flex-row md:items-center justify-between pt-2 gap-6 md:gap-2">
					<div className="flex flex-col md:flex-row md:items-center gap-4 md:gap-1 flex-1 min-w-0">
						<div className="min-w-0 w-full md:w-auto md:max-w-[33%] flex flex-col gap-2 md:gap-0">
							<label
								htmlFor="templateID"
								className="text-xs font-medium text-content-secondary pl-4 md:sr-only"
							>
								Template
							</label>
							<Select
								name="templateID"
								onValueChange={(value) => {
									setSelectedTemplateId(value);
									if (value !== selectedTemplateId) {
										setSelectedPresetId(undefined);
									}
								}}
								defaultValue={templates[0].id}
								required
							>
								<PromptSelectTrigger id="templateID" tooltip="Template">
									<SelectValue placeholder="Select a template" />
								</PromptSelectTrigger>
								<SelectContent>
									{templates.map((template) => {
										return (
											<SelectItem value={template.id} key={template.id}>
												<div className="flex items-center gap-2">
													{template.icon && (
														<img
															src={template.icon}
															alt={template.name}
															className="size-icon-sm flex-shrink-0"
														/>
													)}
													<span className="overflow-hidden text-ellipsis block">
														{template.display_name || template.name}
													</span>
												</div>
											</SelectItem>
										);
									})}
								</SelectContent>
							</Select>
						</div>

						{permissions.updateTemplates && (
							<div className="min-w-0 w-full md:w-auto md:max-w-[33%] flex flex-col gap-2 md:gap-0">
								<label
									htmlFor="versionId"
									className="text-xs font-medium text-content-secondary pl-4 md:sr-only"
								>
									Template version
								</label>
								<TemplateVersionSelect
									templateId={selectedTemplateId}
									activeVersionId={selectedTemplate.active_version_id}
									value={selectedVersionId}
									onValueChange={setSelectedVersionId}
								/>
							</div>
						)}

						<div className="min-w-0 w-full md:w-auto md:max-w-[33%] flex flex-col gap-2 md:gap-0">
							<label
								htmlFor="presetID"
								className="text-xs font-medium text-content-secondary pl-4 md:sr-only"
							>
								Preset
							</label>
							{isLoadingPresets ? (
								<Skeleton className="w-[140px] h-8 rounded-full" />
							) : (
								presets &&
								presets.length > 0 &&
								selectedPresetId && (
									<Select
										key={`preset-select-${selectedTemplate.active_version_id}`}
										name="presetID"
										value={selectedPresetId}
										onValueChange={setSelectedPresetId}
									>
										<PromptSelectTrigger
											id="presetID"
											tooltip="Preset"
											className="max-w-full [&>span]:flex [&>span]:items-center [&>span]:gap-2 [&>span>span]:truncate [&>span>span]:min-w-0 [&_svg[data-slot='preset-description']]:hidden"
										>
											<SelectValue placeholder="Select a preset" />
										</PromptSelectTrigger>
										<SelectContent>
											{presets?.toSorted(sortByDefault).map((preset) => (
												<SelectItem
													value={preset.ID}
													key={preset.ID}
													className="[&_span]:flex [&_span]:items-center [&_span]:gap-2"
												>
													{preset.Icon && (
														<img
															src={preset.Icon}
															alt={preset.Name}
															className="size-icon-sm shrink-0"
														/>
													)}
													<span className="truncate min-w-0">
														{preset.Name}
													</span>
													{preset.Default && (
														<Badge size="xs" className="shrink-0">
															Default
														</Badge>
													)}
													{preset.Description && (
														<Tooltip>
															<TooltipTrigger asChild>
																<InfoIcon
																	className="size-4"
																	data-slot="preset-description"
																/>
															</TooltipTrigger>
															<TooltipContent>
																{preset.Description}
															</TooltipContent>
														</Tooltip>
													)}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								)
							)}
						</div>
					</div>

					<div className="flex items-center gap-2 md:shrink-0">
						{missedExternalAuth && (
							<ExternalAuthButtons
								versionId={selectedVersionId}
								missedExternalAuth={missedExternalAuth}
							/>
						)}

						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									type="submit"
									disabled={prompt.trim().length === 0 || isMissingExternalAuth}
									className="w-full md:w-auto rounded-full disabled:bg-surface-invert-primary disabled:opacity-70 md:size-8 md:min-w-[auto]"
								>
									<span className="md:hidden">Run task</span>
									<Spinner
										loading={
											isLoadingExternalAuth ||
											isPollingExternalAuth ||
											createTaskMutation.isPending
										}
									>
										<ArrowUpIcon className="!size-icon-sm" />
									</Spinner>
								</Button>
							</TooltipTrigger>
							<TooltipContent align="end">
								<KbdGroup>
									<Kbd>{getOSKey()}</Kbd>
									<span>+</span>
									<Kbd>Enter</Kbd>
								</KbdGroup>
							</TooltipContent>
						</Tooltip>
					</div>
				</div>
			</fieldset>
		</form>
	);
};

type ExternalAuthButtonProps = {
	versionId: string;
	missedExternalAuth: TemplateVersionExternalAuth[];
};

const ExternalAuthButtons: FC<ExternalAuthButtonProps> = ({
	versionId,
	missedExternalAuth,
}) => {
	const {
		startPollingExternalAuth,
		isPollingExternalAuth,
		externalAuthPollingState,
	} = useExternalAuth(versionId);
	const shouldRetry = externalAuthPollingState === "abandoned";

	return missedExternalAuth.map((auth) => {
		return (
			<div className="flex items-center gap-2" key={auth.id}>
				<Button
					className="bg-surface-tertiary hover:bg-surface-quaternary rounded-full text-content-primary"
					size="sm"
					disabled={isPollingExternalAuth || auth.authenticated}
					onClick={() => {
						window.open(
							auth.authenticate_url,
							"_blank",
							"width=900,height=600",
						);
						startPollingExternalAuth();
					}}
				>
					<Spinner loading={isPollingExternalAuth}>
						<ExternalImage src={auth.display_icon} />
					</Spinner>
					Connect to {auth.display_name}
				</Button>

				{shouldRetry && !auth.authenticated && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="icon"
								onClick={startPollingExternalAuth}
							>
								<RedoIcon />
								<span className="sr-only">Refresh external auth</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>
							Retry connecting to {auth.display_name}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
		);
	});
};

function sortByDefault(a: Preset, b: Preset) {
	// Default preset should come first
	if (a.Default && !b.Default) return -1;
	if (!a.Default && b.Default) return 1;
	// Otherwise, sort alphabetically by name
	return a.Name.localeCompare(b.Name);
}

// TODO: Enforce task creation to always use the latest active template version.
// During task creation, the active version might change between template load
// and user action. Since handling this in the FE cannot guarantee correctness,
// we should move the logic to the BE after the experimental phase.
async function createTaskWithLatestTemplateVersion(
	input: string,
	userId: string,
	templateId: string,
	presetId: string | undefined,
): Promise<Task> {
	const template = await API.getTemplate(templateId);
	return API.createTask(userId, {
		input,
		template_version_id: template.active_version_id,
		template_version_preset_id: presetId,
	});
}

type PromptTextareaProps = TextareaAutosizeProps & {
	isSubmitting?: boolean;
};

const PromptTextarea: FC<PromptTextareaProps> = ({
	isSubmitting,
	...props
}) => {
	return (
		<div className="relative">
			<TextareaAutosize
				{...props}
				required
				id="prompt"
				name="prompt"
				placeholder="Prompt your AI agent to start a task..."
				className={`border-0 px-3 py-2 resize-none w-full h-full bg-transparent rounded-lg
							outline-none flex min-h-24 text-sm shadow-sm text-content-primary
							placeholder:text-content-secondary md:text-sm ${props.readOnly || isSubmitting ? "opacity-60 cursor-not-allowed" : ""}`}
			/>
			{isSubmitting && (
				<div className="absolute inset-0 pointer-events-none overflow-hidden">
					<div
						className={`absolute top-0 w-0.5 h-full
						bg-green-400/90 animate-caret-scan rounded-sm
						shadow-[-15px_0_15px_rgba(0,255,0,0.9),-30px_0_30px_rgba(0,255,0,0.7),-45px_0_45px_rgba(0,255,0,0.5),-60px_0_60px_rgba(0,255,0,0.3)]`}
					/>
				</div>
			)}
		</div>
	);
};
