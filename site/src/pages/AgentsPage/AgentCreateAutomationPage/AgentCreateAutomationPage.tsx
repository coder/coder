import { useFormik } from "formik";
import { useEffect, useId, useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { Link, Navigate, useNavigate } from "react-router";
import {
	type ApiErrorResponse,
	isApiError,
	mapApiErrorToFieldErrors,
} from "#/api/errors";
import { createChatAutomation } from "#/api/queries/chatAutomations";
import { chatProjects } from "#/api/queries/chatProjects";
import { chatModels, infiniteChats } from "#/api/queries/chats";
import type {
	CreateChatAutomationRequest,
	CreateChatAutomationResponse,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ExperimentalBadge } from "#/components/Badge/PresetBadges";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { timeZones } from "#/utils/timeZones";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { RetryButton } from "../components/RetryButton";
import { SectionHeader } from "../components/SectionHeader";
import { resolveModelSelector } from "../utils/modelOptions";
import {
	AgentCreateAutomationForm,
	type CreateAutomationFormValues,
} from "./AgentCreateAutomationForm";

const validate = (values: CreateAutomationFormValues) => {
	const errors: Partial<Record<keyof CreateAutomationFormValues, string>> = {};
	const name = values.name.trim();
	if (!name) {
		errors.name = "Enter a name.";
	} else if (Array.from(name).length > 128) {
		errors.name = "Use 128 characters or fewer.";
	} else if (name.includes("\0")) {
		errors.name = "The name cannot contain null characters.";
	}
	if (!values.organization_id) {
		errors.organization_id = "Select an organization.";
	}
	if (!values.prompt.trim()) {
		errors.prompt = "Enter a prompt.";
	}
	if (values.target_mode === "new_chat") {
		if (!values.new_chat_model_config_id) {
			errors.new_chat_model_config_id = "Select a model.";
		}
	} else if (!values.target_chat_id) {
		errors.target_chat_id = "Select a chat.";
	}
	if (values.kind === "schedule") {
		const cron = values.schedule_cron.trim();
		if (cron.split(/\s+/).length !== 5) {
			errors.schedule_cron = "Enter a five-field cron expression.";
		} else if (new TextEncoder().encode(cron).length > 256) {
			errors.schedule_cron = "Use a cron expression of 256 bytes or fewer.";
		}
		if (!timeZones.includes(values.schedule_time_zone)) {
			errors.schedule_time_zone = "Select a time zone.";
		}
	} else if (
		values.webhook_auth === "standard_webhooks" &&
		!values.webhook_secret
	) {
		errors.webhook_secret = "Enter your Standard Webhooks signing secret.";
	}
	return errors;
};

type AutomationCreatedProps = {
	response: CreateChatAutomationResponse;
	secretSaved: boolean;
	onSecretSavedChange: (saved: boolean) => void;
	ref: React.Ref<HTMLElement>;
};

const AutomationCreated: React.FC<AutomationCreatedProps> = ({
	response,
	secretSaved,
	onSecretSavedChange,
	ref,
}) => {
	const secretSavedId = useId();
	return (
		<section
			ref={ref}
			tabIndex={-1}
			aria-label="Automation created"
			className="mx-auto flex w-full max-w-2xl flex-col gap-6"
		>
			<SectionHeader
				label="Automation created"
				description={`${response.automation.name} has been saved.`}
			/>
			{response.webhook_secret && (
				<div className="flex flex-col gap-4">
					<p className="m-0 text-sm text-content-secondary">
						Save this webhook secret now. It is shown only once and cannot be
						retrieved from the automations list.
					</p>
					<CodeExample
						code={response.webhook_secret}
						secret={false}
						redactPattern={/.+/g}
						showRevealButton
					/>
					<label
						htmlFor={secretSavedId}
						className="flex items-start gap-2 text-sm"
					>
						<Checkbox
							id={secretSavedId}
							checked={secretSaved}
							onCheckedChange={(checked) =>
								onSecretSavedChange(checked === true)
							}
						/>
						I have saved this secret
					</label>
				</div>
			)}
			<Button asChild className="self-start">
				<Link to="/agents/automations">View automations</Link>
			</Button>
		</section>
	);
};

const AgentCreateAutomationPage: React.FC = () => {
	const { organizations, experiments } = useDashboard();
	const { user } = useAuthenticated();
	const enabled = experiments.includes("chat-automations");
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createMutation = useMutation(createChatAutomation(queryClient));
	const submissionInFlight = useRef(false);
	const [submitError, setSubmitError] = useState<ApiErrorResponse>();
	const [created, setCreated] = useState<CreateChatAutomationResponse>();
	const [secretSaved, setSecretSaved] = useState(false);
	const successRef = useRef<HTMLElement>(null);
	const leaveTriggerRef = useRef<HTMLElement | null>(null);
	const form = useFormik<CreateAutomationFormValues>({
		initialValues: {
			organization_id:
				getDefaultOrganizationId(organizations) ?? organizations[0]?.id ?? "",
			name: "",
			prompt: "",
			kind: "schedule",
			target_mode: "new_chat",
			enabled: true,
			schedule_cron: "0 9 * * 1-5",
			schedule_time_zone: "UTC",
			when_busy: "skip",
			webhook_auth: "bearer",
			webhook_secret: "",
			target_chat_id: "",
			new_chat_model_config_id: "",
			project_id: "",
			reasoning_effort: "",
		},
		validate,
		onSubmit: (values, helpers) => {
			if (submissionInFlight.current) {
				return;
			}
			const errors: Partial<Record<keyof CreateAutomationFormValues, string>> =
				{};
			if (!enabled || !organization) {
				errors.organization_id = "Select an accessible organization.";
			}
			if (values.target_mode === "new_chat") {
				const model = modelOptions.find(
					(option) => option.id === values.new_chat_model_config_id,
				);
				if (!model) {
					errors.new_chat_model_config_id =
						"Select an available model in this organization.";
				}
				if (
					values.reasoning_effort &&
					!model?.reasoningEfforts?.includes(values.reasoning_effort)
				) {
					errors.reasoning_effort = "Select an effort supported by this model.";
				}
				if (
					values.project_id &&
					!projects.some((project) => project.id === values.project_id)
				) {
					errors.project_id = "Select an available project or None.";
				}
			} else if (!chats.some((chat) => chat.id === values.target_chat_id)) {
				errors.target_chat_id =
					"Select one of your active chats in this organization.";
			}
			if (Object.keys(errors).length > 0) {
				helpers.setErrors(errors);
				helpers.setSubmitting(false);
				return;
			}

			const req: CreateChatAutomationRequest = {
				name: values.name.trim(),
				prompt: values.prompt,
				enabled: values.enabled,
				kind: values.kind,
				target_mode: values.target_mode,
				...(values.kind === "schedule"
					? {
							schedule_cron: values.schedule_cron.trim(),
							schedule_time_zone: values.schedule_time_zone,
						}
					: {
							webhook_auth: values.webhook_auth,
							...(values.webhook_auth === "standard_webhooks" && {
								webhook_secret: values.webhook_secret,
							}),
						}),
				...(values.target_mode === "new_chat"
					? {
							new_chat_model_config_id: values.new_chat_model_config_id,
							...(values.project_id && { project_id: values.project_id }),
							...(values.reasoning_effort && {
								reasoning_effort: values.reasoning_effort,
							}),
						}
					: {
							target_chat_id: values.target_chat_id,
							when_busy: values.when_busy,
						}),
			};
			submissionInFlight.current = true;
			setSubmitError(undefined);
			createMutation.mutate(
				{ organizationId: values.organization_id, req },
				{
					onSuccess: (response) => {
						setCreated(response);
						helpers.resetForm();
					},
					onError: (error) => {
						// Axios errors retain the request, including a supplied signing secret.
						if (isApiError(error)) {
							helpers.setErrors(mapApiErrorToFieldErrors(error.response.data));
							setSubmitError({
								message: error.response.data.message,
								detail: error.response.data.detail,
							});
						} else {
							setSubmitError({
								message: "Could not confirm automation creation.",
								detail:
									"Check the automations list before retrying. The request may have succeeded.",
							});
						}
					},
					onSettled: () => {
						submissionInFlight.current = false;
						helpers.setSubmitting(false);
						createMutation.reset();
					},
				},
			);
		},
	});
	const organization = organizations.find(
		(item) => item.id === form.values.organization_id,
	);
	const selectingNewChat = form.values.target_mode === "new_chat";
	const canLoad = enabled && Boolean(organization) && !created;
	const modelsQuery = useQuery({
		...chatModels(form.values.organization_id),
		enabled: canLoad && selectingNewChat,
	});
	const { options: modelOptions } = resolveModelSelector(
		form.values.organization_id,
		modelsQuery,
	);
	const projectsQuery = useQuery({
		...chatProjects(),
		enabled: canLoad && selectingNewChat,
	});
	const projects = (projectsQuery.data ?? []).filter(
		(project) => project.organization_id === form.values.organization_id,
	);
	const chatsQuery = useInfiniteQuery({
		...infiniteChats({ sources: ["created_by_me"] }),
		enabled: canLoad && !selectingNewChat,
	});
	const chats = (chatsQuery.data?.pages.flat() ?? []).filter(
		(chat) =>
			chat.organization_id === form.values.organization_id &&
			chat.owner_id === user.id &&
			!chat.parent_chat_id &&
			!chat.archived,
	);
	const isPending = createMutation.isPending || form.isSubmitting;
	const leavePrompt = useUnsavedChangesPrompt(
		enabled &&
			(isPending ||
				(!created && form.dirty) ||
				(Boolean(created?.webhook_secret) && !secretSaved)),
	);

	useEffect(() => {
		if (created && !leavePrompt.isOpen) {
			successRef.current?.focus();
		}
	}, [created, leavePrompt.isOpen]);

	if (!enabled) {
		return <Navigate to="/agents" replace />;
	}

	let leaveTitle = "Leave this page?";
	let leaveDescription = "Your automation has not been saved. Leave anyway?";
	if (isPending) {
		leaveTitle = "Creating automation";
		leaveDescription =
			"Wait for creation to finish before leaving so you do not lose a one-time webhook secret.";
	} else if (created?.webhook_secret) {
		leaveTitle = "Leave without saving the secret?";
		leaveDescription =
			"The webhook secret cannot be retrieved again. Save it before leaving this page.";
	}

	const resourceQueries = selectingNewChat
		? [
				{ name: "models", query: modelsQuery },
				{ name: "projects", query: projectsQuery },
			]
		: [{ name: "chats", query: chatsQuery }];

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
			<title>{pageTitle("Create automation", "Agents")}</title>
			<AgentPageHeader
				mobileBack={{ to: "/agents/automations", label: "Automations" }}
			/>
			<div className="p-4 pt-8 pb-12">
				{created ? (
					<AutomationCreated
						ref={successRef}
						response={created}
						secretSaved={secretSaved}
						onSecretSavedChange={setSecretSaved}
					/>
				) : (
					<section className="mx-auto flex w-full max-w-2xl flex-col gap-8">
						<SectionHeader
							label="Create automation"
							description="Configure a scheduled prompt or webhook for an agent."
							badge={<ExperimentalBadge />}
						/>
						<p className="m-0 text-sm text-content-secondary">
							This preview saves automation configurations. Scheduled execution
							and webhook delivery are not available yet.
						</p>
						{organizations.length === 0 ? (
							<div className="flex flex-col items-start gap-4">
								<p className="m-0 text-sm text-content-secondary">
									You need access to an organization to create an automation.
								</p>
								<Button asChild variant="outline">
									<Link to="/agents/automations">Back to automations</Link>
								</Button>
							</div>
						) : (
							<>
								{resourceQueries.map(({ name, query }) =>
									query.error ? (
										<ErrorAlert
											key={name}
											error={query.error}
											showDebugDetail={false}
											actions={
												!isPending && (
													<RetryButton
														aria-label={`Retry loading ${name}`}
														isRetrying={query.isFetching}
														onRetry={() => void query.refetch()}
													/>
												)
											}
										/>
									) : null,
								)}
								<AgentCreateAutomationForm
									form={form}
									organizations={organizations}
									modelOptions={modelOptions}
									projects={projects}
									chats={chats}
									loadingModels={modelsQuery.isLoading}
									loadingProjects={projectsQuery.isLoading}
									loadingChats={chatsQuery.isLoading}
									hasMoreChats={chatsQuery.hasNextPage}
									loadingMoreChats={chatsQuery.isFetchingNextPage}
									onLoadMoreChats={() => void chatsQuery.fetchNextPage()}
									isPending={isPending}
									submitError={submitError}
									onCancel={() => navigate("/agents/automations")}
								/>
							</>
						)}
					</section>
				)}
			</div>
			<ConfirmDialog
				open={leavePrompt.isOpen}
				title={leaveTitle}
				description={leaveDescription}
				hideCancel={false}
				cancelText="Stay"
				confirmText="Leave"
				disabled={isPending}
				onClose={leavePrompt.onCancel}
				onConfirm={leavePrompt.onConfirm}
				onOpenAutoFocus={() => {
					leaveTriggerRef.current =
						document.activeElement instanceof HTMLElement
							? document.activeElement
							: null;
				}}
				onCloseAutoFocus={(event) => {
					const target = leaveTriggerRef.current?.isConnected
						? leaveTriggerRef.current
						: successRef.current;
					if (target) {
						event.preventDefault();
						target.focus();
					}
				}}
			/>
		</ScrollArea>
	);
};

export default AgentCreateAutomationPage;
