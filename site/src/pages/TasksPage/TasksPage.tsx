import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { disabledRefetchOptions } from "api/queries/util";
import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { useAuthenticated } from "hooks";
import { ExternalLinkIcon, RotateCcwIcon, SendIcon } from "lucide-react";
import { AI_PROMPT_PARAMETER_NAME, type Task } from "modules/tasks/tasks";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import { type FC, type ReactNode, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router-dom";
import TextareaAutosize from "react-textarea-autosize";
import { pageTitle } from "utils/page";
import { relativeTime } from "utils/time";
import { type UserOption, UsersCombobox } from "./UsersCombobox";

type TasksFilter = {
	user: UserOption | undefined;
};

const TasksPage: FC = () => {
	const {
		data: templates,
		error,
		refetch,
	} = useQuery({
		queryKey: ["templates", "ai"],
		queryFn: data.fetchAITemplates,
		...disabledRefetchOptions,
	});
	const { user, permissions } = useAuthenticated();
	const [filter, setFilter] = useState<TasksFilter>({
		user: {
			value: user.username,
			label: user.name || user.username,
			avatarUrl: user.avatar_url,
		},
	});

	let content: ReactNode = null;

	if (error) {
		const message = getErrorMessage(error, "Error loading AI templates");
		const detail = getErrorDetail(error) ?? "Please, try again";

		content = (
			<div className="border border-solid rounded-lg w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						{message}
					</h3>
					<span className="text-content-secondary text-sm">{detail}</span>
					<Button size="sm" onClick={() => refetch()} className="mt-4">
						<RotateCcwIcon />
						Try again
					</Button>
				</div>
			</div>
		);
	} else if (templates) {
		content =
			templates.length === 0 ? (
				<div className="rounded-lg border border-solid border-border w-full min-h-80 p-4 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							No AI templates found
						</h3>
						<span className="text-content-secondary text-sm">
							Create an AI template to get started
						</span>
						<Button size="sm" className="mt-4">
							<ExternalLinkIcon />
							Read the docs
						</Button>
					</div>
				</div>
			) : (
				<>
					<TaskForm templates={templates} />
					{permissions.viewDeploymentConfig && (
						<TasksFilter filter={filter} onFilterChange={setFilter} />
					)}
					<TasksTable templates={templates} filter={filter} />
				</>
			);
	} else {
		content = (
			<div className="rounded-lg border border-solid border-border w-full min-h-80 p-4 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<Spinner loading className="mb-4" />
					<h3 className="m-0 font-medium text-content-primary text-base">
						Loading AI templates
					</h3>
					<span className="text-content-secondary text-sm">
						This might take a few minutes
					</span>
				</div>
			</div>
		);
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle("AI Tasks")}</title>
			</Helmet>
			<Margins>
				<PageHeader
					actions={
						<Button variant="outline">
							<ExternalLinkIcon />
							Read the docs
						</Button>
					}
				>
					<PageHeaderTitle>Tasks</PageHeaderTitle>
					<PageHeaderSubtitle>Automate tasks with AI</PageHeaderSubtitle>
				</PageHeader>

				<main className="pb-8">{content}</main>
			</Margins>
		</>
	);
};

type CreateTaskMutationFnProps = { prompt: string; templateId: string };

type TaskFormProps = {
	templates: Template[];
};

const TaskForm: FC<TaskFormProps> = ({ templates }) => {
	const { user } = useAuthenticated();
	const queryClient = useQueryClient();

	const createTaskMutation = useMutation({
		mutationFn: async ({ prompt, templateId }: CreateTaskMutationFnProps) =>
			data.createTask(prompt, user.id, templateId),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: ["tasks"],
			});
		},
	});

	const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
		e.preventDefault();

		const form = e.currentTarget;
		const formData = new FormData(form);
		const prompt = formData.get("prompt") as string;
		const templateID = formData.get("templateID") as string;

		if (!prompt || !templateID) {
			return;
		}

		try {
			await createTaskMutation.mutateAsync({
				prompt,
				templateId: templateID,
			});
			form.reset();
		} catch (error) {
			const message = getErrorMessage(error, "Error creating task");
			const detail = getErrorDetail(error) ?? "Please, try again";
			displayError(message, detail);
		}
	};

	return (
		<form
			className="border border-border border-solid rounded-lg p-4"
			onSubmit={onSubmit}
			aria-label="Create AI task"
		>
			<fieldset disabled={createTaskMutation.isPending}>
				<label htmlFor="prompt" className="sr-only">
					Prompt
				</label>
				<TextareaAutosize
					required
					id="prompt"
					name="prompt"
					placeholder="Prompt your AI agent to start a task..."
					className={`border-0 resize-none w-full h-full bg-transparent rounded-lg outline-none flex min-h-[60px]
						text-sm shadow-sm text-content-primary placeholder:text-content-secondary md:text-sm`}
				/>
				<div className="flex items-center justify-between pt-2">
					<Select name="templateID" defaultValue={templates[0].id} required>
						<SelectTrigger className="w-52 text-xs [&_svg]:size-icon-xs border-0 bg-surface-secondary h-8 px-3">
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

					<Button size="sm" type="submit">
						<Spinner loading={createTaskMutation.isPending}>
							<SendIcon />
						</Spinner>
						Run task
					</Button>
				</div>
			</fieldset>
		</form>
	);
};

type TasksFilterProps = {
	filter: TasksFilter;
	onFilterChange: (filter: TasksFilter) => void;
};

const TasksFilter: FC<TasksFilterProps> = ({ filter, onFilterChange }) => {
	return (
		<section className="mt-6" aria-labelledby="filters-title">
			<h3 id="filters-title" className="sr-only">
				Filters
			</h3>
			<UsersCombobox
				selectedOption={filter.user}
				onSelect={(userOption) =>
					onFilterChange({
						...filter,
						user: userOption,
					})
				}
			/>
		</section>
	);
};

type TasksTableProps = {
	templates: Template[];
	filter: TasksFilter;
};

const TasksTable: FC<TasksTableProps> = ({ templates, filter }) => {
	const {
		data: tasks,
		error,
		refetch,
	} = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => data.fetchTasks(templates, filter),
		refetchInterval: 10_000,
	});

	let body: ReactNode = null;

	if (error) {
		const message = getErrorMessage(error, "Error loading tasks");
		const detail = getErrorDetail(error) ?? "Please, try again";

		body = (
			<TableRow>
				<TableCell colSpan={4} className="text-center">
					<div className="rounded-lg w-full min-h-80 flex items-center justify-center">
						<div className="flex flex-col items-center">
							<h3 className="m-0 font-medium text-content-primary text-base">
								{message}
							</h3>
							<span className="text-content-secondary text-sm">{detail}</span>
							<Button size="sm" onClick={() => refetch()} className="mt-4">
								<RotateCcwIcon />
								Try again
							</Button>
						</div>
					</div>
				</TableCell>
			</TableRow>
		);
	} else if (tasks) {
		body =
			tasks.length === 0 ? (
				<TableRow>
					<TableCell colSpan={4} className="text-center">
						<div className="w-full min-h-80 p-4 flex items-center justify-center">
							<div className="flex flex-col items-center">
								<h3 className="m-0 font-medium text-content-primary text-base">
									No tasks found
								</h3>
								<span className="text-content-secondary text-sm">
									Use the form above to run a task
								</span>
							</div>
						</div>
					</TableCell>
				</TableRow>
			) : (
				tasks.map(({ workspace, prompt }) => {
					const templateDisplayName =
						workspace.template_display_name ?? workspace.template_name;
					const status = workspace.latest_app_status;
					const agent = workspace.latest_build.resources
						.flatMap((r) => r.agents)
						.find((a) => a?.id === status?.agent_id);
					const app = agent?.apps.find((a) => a.id === status?.app_id);

					return (
						<TableRow key={workspace.id} className="relative" hover>
							<TableCell>
								<AvatarData
									title={
										<>
											<span className="block max-w-[520px] overflow-hidden text-ellipsis whitespace-nowrap">
												{prompt}
											</span>
											<RouterLink
												to={`/tasks/${workspace.owner_name}/${workspace.name}`}
												className="absolute inset-0"
											>
												<span className="sr-only">Access task</span>
											</RouterLink>
										</>
									}
									subtitle={templateDisplayName}
									avatar={
										<Avatar
											size="lg"
											variant="icon"
											src={workspace.template_icon}
											fallback={templateDisplayName}
										/>
									}
								/>
							</TableCell>
							<TableCell>
								<WorkspaceAppStatus
									disabled={workspace.latest_build.status !== "running"}
									status={workspace.latest_app_status}
								/>
							</TableCell>
							<TableCell>
								<AvatarData
									title={workspace.owner_name}
									subtitle={
										<span className="block first-letter:uppercase">
											{relativeTime(new Date(workspace.created_at))}
										</span>
									}
									src={workspace.owner_avatar_url}
								/>
							</TableCell>
						</TableRow>
					);
				})
			);
	} else {
		body = (
			<TableRow>
				<TableCell colSpan={4}>
					<div className="rounded-lg w-full min-h-80 flex items-center justify-center">
						<div className="flex flex-col items-center">
							<Spinner loading className="mb-4" />
							<h3 className="m-0 font-medium text-content-primary text-base">
								Loading tasks
							</h3>
							<span className="text-content-secondary text-sm">
								This might take a few minutes
							</span>
						</div>
					</div>
				</TableCell>
			</TableRow>
		);
	}

	return (
		<Table className="mt-4">
			<TableHeader>
				<TableRow>
					<TableHead>Task</TableHead>
					<TableHead>Status</TableHead>
					<TableHead>Created by</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>{body}</TableBody>
		</Table>
	);
};

export const data = {
	// TODO: This function is currently inefficient because it fetches all templates
	// and their parameters individually, resulting in many API calls and slow
	// performance. After confirming the requirements, consider adding a backend
	// endpoint that returns only AI templates (those with an "AI Prompt" parameter)
	// in a single request.
	async fetchAITemplates() {
		const templates = await API.getTemplates();
		const parameters = await Promise.all(
			templates.map(async (template) =>
				API.getTemplateVersionRichParameters(template.active_version_id),
			),
		);
		return templates.filter((_template, index) => {
			return parameters[index].some((p) => p.name === AI_PROMPT_PARAMETER_NAME);
		});
	},

	// TODO: This function is inefficient because it fetches workspaces for each
	// template individually and its build parameters resulting in excessive API
	// calls and slow performance. Consider implementing a backend endpoint that
	// returns all AI-related workspaces in a single request to improve efficiency.
	async fetchTasks(aiTemplates: Template[], filter: TasksFilter) {
		const workspaces = await Promise.all(
			aiTemplates.map((template) => {
				const queryParts = [`template:${template.name}`];
				if (filter.user) {
					queryParts.push(`owner:${filter.user.value}`);
				}

				return API.getWorkspaces({
					q: queryParts.join(" "),
					limit: 100,
				});
			}),
		).then((results) =>
			results
				.flatMap((r) => r.workspaces)
				.toSorted((a, b) => {
					return (
						new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
					);
				}),
		);

		return Promise.all(
			workspaces.map(async (workspace) => {
				const parameters = await API.getWorkspaceBuildParameters(
					workspace.latest_build.id,
				);
				const prompt = parameters.find(
					(p) => p.name === AI_PROMPT_PARAMETER_NAME,
				)?.value;

				if (!prompt) {
					return;
				}

				return {
					workspace,
					prompt,
				} satisfies Task;
			}),
		).then((tasks) => tasks.filter((t) => t !== undefined));
	},

	async createTask(
		prompt: string,
		userId: string,
		templateId: string,
	): Promise<Task> {
		const workspace = await API.createWorkspace(userId, {
			name: `task-${new Date().getTime()}`,
			template_id: templateId,
			rich_parameter_values: [
				{ name: AI_PROMPT_PARAMETER_NAME, value: prompt },
			],
		});

		return {
			workspace,
			prompt,
		};
	},
};

export default TasksPage;
