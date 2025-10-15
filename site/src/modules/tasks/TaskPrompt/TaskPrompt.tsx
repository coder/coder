import type { SelectTriggerProps } from "@radix-ui/react-select";
import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	templateVersionPresets,
	templateVersions,
} from "api/queries/templates";
import type {
	Preset,
	Task,
	Template,
	TemplateVersionExternalAuth,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Link } from "components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Skeleton } from "components/Skeleton/Skeleton";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks/useAuthenticated";
import { useExternalAuth } from "hooks/useExternalAuth";
import { ArrowUpIcon, RedoIcon, RotateCcwIcon } from "lucide-react";
import { AI_PROMPT_PARAMETER_NAME } from "modules/tasks/tasks";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import TextareaAutosize, {
	type TextareaAutosizeProps,
} from "react-textarea-autosize";
import { cn } from "utils/cn";
import { docs } from "utils/docs";

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

	const {
		externalAuth,
		externalAuthError,
		isPollingExternalAuth,
		isLoadingExternalAuth,
	} = useExternalAuth(selectedTemplate.active_version_id);

	// Template versions
	const [selectedVersionId, setSelectedVersionId] = useState(
		selectedTemplate.active_version_id,
	);
	const versionsQuery = useQuery({
		...templateVersions(selectedTemplate.id),
		enabled: permissions.updateTemplates,
	});
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
	const selectedPreset = presets?.find((p) => p.ID === selectedPresetId);

	// Read-only prompt if defined in preset
	const presetPrompt = selectedPreset?.Parameters?.find(
		(param) => param.Name === AI_PROMPT_PARAMETER_NAME,
	)?.Value;
	const isPromptReadOnly = !!presetPrompt;
	useEffect(() => {
		if (presetPrompt) {
			setPrompt(presetPrompt);
		}
	}, [presetPrompt]);

	// External Auth
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
				return API.experimental.createTask(user.id, {
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

	const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();

		try {
			await createTaskMutation.mutateAsync({
				prompt,
			});
		} catch (error) {
			const message = getErrorMessage(error, "Error creating task");
			const detail = getErrorDetail(error) ?? "Please try again";
			displayError(message, detail);
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
				className="border border-border border-solid rounded-3xl p-3 bg-surface-secondary"
				disabled={createTaskMutation.isPending}
			>
				<label
					htmlFor="prompt"
					className={
						isPromptReadOnly
							? "text-xs font-medium text-content-primary block px-3 pt-2"
							: "sr-only"
					}
				>
					{isPromptReadOnly ? "Prompt defined by preset" : "Prompt"}
				</label>
				<PromptTextarea
					required
					value={prompt}
					onChange={(e) => setPrompt(e.target.value)}
					readOnly={isPromptReadOnly}
				/>
				<div className="flex items-center justify-between pt-2">
					<div className="flex items-center gap-1">
						<div>
							<label htmlFor="templateID" className="sr-only">
								Select template
							</label>
							<Select
								name="templateID"
								onValueChange={(value) => setSelectedTemplateId(value)}
								defaultValue={templates[0].id}
								required
							>
								<PromptSelectTrigger id="templateID">
									<SelectValue placeholder="Select a template" />
								</PromptSelectTrigger>
								<SelectContent>
									{templates.map((template) => {
										return (
											<SelectItem value={template.id} key={template.id}>
												<span className="overflow-hidden text-ellipsis block">
													{template.display_name || template.name}
												</span>
											</SelectItem>
										);
									})}
								</SelectContent>
							</Select>
						</div>

						{versionsQuery.data && (
							<div>
								<label htmlFor="versionId" className="sr-only">
									Template version
								</label>
								<Select
									name="versionId"
									onValueChange={(value) => setSelectedVersionId(value)}
									value={selectedVersionId}
									required
								>
									<PromptSelectTrigger id="versionId">
										<SelectValue placeholder="Select a version" />
									</PromptSelectTrigger>
									<SelectContent>
										{versionsQuery.data.map((version) => {
											return (
												<SelectItem value={version.id} key={version.id}>
													{version.name}
												</SelectItem>
											);
										})}
									</SelectContent>
								</Select>
							</div>
						)}

						<div>
							<label htmlFor="presetID" className="sr-only">
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
										<PromptSelectTrigger id="presetID">
											<SelectValue placeholder="Select a preset" />
										</PromptSelectTrigger>
										<SelectContent>
											{presets?.toSorted(sortByDefault).map((preset) => (
												<SelectItem value={preset.ID} key={preset.ID}>
													<span className="overflow-hidden text-ellipsis block">
														{preset.Name} {preset.Default && "(Default)"}
													</span>
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								)
							)}
						</div>
					</div>

					<div className="flex items-center gap-2">
						{missedExternalAuth && (
							<ExternalAuthButtons
								template={selectedTemplate}
								missedExternalAuth={missedExternalAuth}
							/>
						)}

						<Button
							size="icon"
							type="submit"
							disabled={isMissingExternalAuth}
							className="rounded-full disabled:bg-surface-invert-primary disabled:opacity-70"
						>
							<Spinner
								loading={
									isLoadingExternalAuth ||
									isPollingExternalAuth ||
									createTaskMutation.isPending
								}
							>
								<ArrowUpIcon />
							</Spinner>
							<span className="sr-only">Run task</span>
						</Button>
					</div>
				</div>
			</fieldset>
		</form>
	);
};

const PromptSelectTrigger: FC<SelectTriggerProps> = ({
	className,
	...props
}) => {
	return (
		<SelectTrigger
			{...props}
			className={cn([
				className,
				`border-0 bg-surface-secondary text-sm text-content-primary gap-2 px-3
				[&_svg]:text-inherit cursor-pointer hover:bg-surface-quaternary rounded-full
				h-8 data-[state=open]:bg-surface-tertiary`,
			])}
		/>
	);
};

type ExternalAuthButtonProps = {
	template: Template;
	missedExternalAuth: TemplateVersionExternalAuth[];
};

const ExternalAuthButtons: FC<ExternalAuthButtonProps> = ({
	template,
	missedExternalAuth,
}) => {
	const {
		startPollingExternalAuth,
		isPollingExternalAuth,
		externalAuthPollingState,
	} = useExternalAuth(template.active_version_id);
	const shouldRetry = externalAuthPollingState === "abandoned";

	return missedExternalAuth.map((auth) => {
		return (
			<div className="flex items-center gap-2" key={auth.id}>
				<Button
					className="bg-surface-tertiary hover:bg-surface-quaternary rounded-full text-white"
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
					<TooltipProvider>
						<Tooltip delayDuration={100}>
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
					</TooltipProvider>
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
	return API.experimental.createTask(userId, {
		input,
		template_version_id: template.active_version_id,
		template_version_preset_id: presetId,
	});
}

const PromptTextarea: FC<TextareaAutosizeProps> = (props) => {
	return (
		<TextareaAutosize
			{...props}
			required
			id="prompt"
			name="prompt"
			placeholder="Prompt your AI agent to start a task..."
			className={`border-0 px-3 py-2 resize-none w-full h-full bg-transparent rounded-lg
						outline-none flex min-h-24 text-sm shadow-sm text-content-primary
						placeholder:text-content-secondary md:text-sm ${props.readOnly ? "opacity-60 cursor-not-allowed" : ""}`}
		/>
	);
};
