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
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { displayError } from "components/GlobalSnackbar/utils";
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
import { RedoIcon, RotateCcwIcon, SendIcon } from "lucide-react";
import { AI_PROMPT_PARAMETER_NAME } from "modules/tasks/tasks";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import { docs } from "utils/docs";

const textareaPlaceholder = "Prompt your AI agent to start a task...";

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
	const navigate = useNavigate();

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
			onSuccess={(task) => {
				navigate(`/tasks/${task.owner_name}/${task.name}`);
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
		<div className="border border-border border-solid rounded-lg p-4">
			<div className="space-y-4">
				{/* Textarea skeleton */}
				<TextareaAutosize
					disabled
					id="prompt"
					name="prompt"
					placeholder={textareaPlaceholder}
					className={`border-0 resize-none w-full h-full bg-transparent rounded-lg outline-none flex min-h-[60px]
				text-sm shadow-sm text-content-primary placeholder:text-content-secondary md:text-sm`}
				/>

				{/* Bottom controls skeleton */}
				<div className="flex items-center justify-between pt-2">
					<Skeleton className="w-[208px] h-8" />
					<Skeleton className="w-[96px] h-8" />
				</div>
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
	const { user } = useAuthenticated();
	const queryClient = useQueryClient();
	const [selectedTemplateId, setSelectedTemplateId] = useState<string>(
		templates[0].id,
	);
	const [selectedPresetId, setSelectedPresetId] = useState<string>();
	const selectedTemplate = templates.find(
		(t) => t.id === selectedTemplateId,
	) as Template;

	const {
		externalAuth,
		externalAuthError,
		isPollingExternalAuth,
		isLoadingExternalAuth,
	} = useExternalAuth(selectedTemplate.active_version_id);

	// Fetch presets when template changes
	const { data: presets, isLoading: isLoadingPresets } = useQuery(
		templateVersionPresets(selectedTemplate.active_version_id),
	);
	const defaultPreset = presets?.find((p) => p.Default);

	// Handle preset selection when data changes
	useEffect(() => {
		setSelectedPresetId(defaultPreset?.ID);
	}, [defaultPreset?.ID]);

	// Extract AI prompt from selected preset
	const selectedPreset = presets?.find((p) => p.ID === selectedPresetId);
	const presetAIPrompt = selectedPreset?.Parameters?.find(
		(param) => param.Name === AI_PROMPT_PARAMETER_NAME,
	)?.Value;
	const isPromptReadOnly = !!presetAIPrompt;

	const missedExternalAuth = externalAuth?.filter(
		(auth) => !auth.optional && !auth.authenticated,
	);
	const isMissingExternalAuth = missedExternalAuth
		? missedExternalAuth.length > 0
		: true;

	const createTaskMutation = useMutation({
		mutationFn: async ({ prompt }: CreateTaskMutationFnProps) =>
			createTaskWithLatestTemplateVersion(
				prompt,
				user.id,
				selectedTemplate.id,
				selectedPresetId,
			),
		onSuccess: async (task) => {
			await queryClient.invalidateQueries({
				queryKey: ["tasks"],
			});
			onSuccess(task);
		},
	});

	const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();

		const form = e.currentTarget;
		const formData = new FormData(form);
		const prompt = presetAIPrompt || (formData.get("prompt") as string);

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
				className="border border-border border-solid rounded-lg p-4"
				disabled={createTaskMutation.isPending}
			>
				<label
					htmlFor="prompt"
					className={
						isPromptReadOnly
							? "text-xs font-medium text-content-primary mb-2 block"
							: "sr-only"
					}
				>
					{isPromptReadOnly ? "Prompt defined by preset" : "Prompt"}
				</label>
				<TextareaAutosize
					required
					id="prompt"
					name="prompt"
					value={presetAIPrompt || undefined}
					readOnly={isPromptReadOnly}
					placeholder={textareaPlaceholder}
					className={`border-0 resize-none w-full h-full bg-transparent rounded-lg outline-none flex min-h-[60px]
						text-sm shadow-sm text-content-primary placeholder:text-content-secondary md:text-sm ${
							isPromptReadOnly ? "opacity-60 cursor-not-allowed" : ""
						}`}
				/>
				<div className="flex items-center justify-between pt-2">
					<div className="flex items-center gap-4">
						<div className="flex flex-col gap-1">
							<label
								htmlFor="templateID"
								className="text-xs font-medium text-content-primary"
							>
								Template
							</label>
							<Select
								name="templateID"
								onValueChange={(value) => setSelectedTemplateId(value)}
								defaultValue={templates[0].id}
								required
							>
								<SelectTrigger
									id="templateID"
									className="w-80 text-xs [&_svg]:size-icon-xs border-0 bg-surface-secondary h-8 px-3"
								>
									<SelectValue placeholder="Select a template" />
								</SelectTrigger>
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

						{isLoadingPresets ? (
							<div className="flex flex-col gap-1">
								<label
									htmlFor="presetID"
									className="text-xs font-medium text-content-primary"
								>
									Preset
								</label>
								<Skeleton className="w-[320px] h-8" />
							</div>
						) : (
							presets &&
							presets.length > 0 && (
								<div className="flex flex-col gap-1">
									<label
										htmlFor="presetID"
										className="text-xs font-medium text-content-primary"
									>
										Preset
									</label>
									<Select
										key={`preset-select-${selectedTemplate.active_version_id}`}
										name="presetID"
										value={selectedPresetId || undefined}
										onValueChange={setSelectedPresetId}
									>
										<SelectTrigger
											id="presetID"
											className="w-80 text-xs [&_svg]:size-icon-xs border-0 bg-surface-secondary h-8 px-3"
										>
											<SelectValue placeholder="Select a preset" />
										</SelectTrigger>
										<SelectContent>
											{presets.toSorted(sortByDefault).map((preset) => (
												<SelectItem value={preset.ID} key={preset.ID}>
													<span className="overflow-hidden text-ellipsis block">
														{preset.Name} {preset.Default && "(Default)"}
													</span>
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</div>
							)
						)}
					</div>

					<div className="flex items-center gap-2">
						{missedExternalAuth && (
							<ExternalAuthButtons
								template={selectedTemplate}
								missedExternalAuth={missedExternalAuth}
							/>
						)}

						<Button size="sm" type="submit" disabled={isMissingExternalAuth}>
							<Spinner
								loading={
									isLoadingExternalAuth ||
									isPollingExternalAuth ||
									createTaskMutation.isPending
								}
							>
								<SendIcon />
							</Spinner>
							Run task
						</Button>
					</div>
				</div>
			</fieldset>
		</form>
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
					variant="outline"
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
	prompt: string,
	userId: string,
	templateId: string,
	presetId: string | undefined,
): Promise<Task> {
	const template = await API.getTemplate(templateId);
	return API.experimental.createTask(userId, {
		prompt,
		template_version_id: template.active_version_id,
		template_version_preset_id: presetId,
	});
}
