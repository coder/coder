import {
	TooltipProvider,
	TooltipTrigger,
	TooltipContent,
	Tooltip,
} from "components/Tooltip/Tooltip";
import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { disabledRefetchOptions } from "api/queries/util";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
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
import { ExternalLinkIcon, RotateCcwIcon, SendIcon } from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import type { FC, PropsWithChildren, ReactNode } from "react";
import { useQuery } from "react-query";
import { relativeTime } from "utils/time";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";

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
					<TasksTable templates={templates} />
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

				{content}
			</Margins>
		</>
	);
};

type TaskFormProps = {
	templates: Template[];
};

const TaskForm: FC<TaskFormProps> = ({ templates }) => {
	return (
		<form className="border border-border border-solid rounded-lg p-4">
			<textarea
				name="prompt"
				placeholder="Write an action for your AI agent to perform..."
				className={`border-0 resize-none w-full h-full bg-transparent rounded-lg outline-none flex min-h-[60px]
						text-sm shadow-sm text-content-primary placeholder:text-content-secondary md:text-sm`}
			/>
			<div className="flex items-center justify-between">
				<Select name="templateID">
					<SelectTrigger className="w-52 text-xs [&_svg]:size-icon-xs border-0 bg-surface-secondary h-8 px-3">
						<SelectValue placeholder="Select a template" />
					</SelectTrigger>
					<SelectContent>
						{templates.map((template) => {
							return (
								<SelectItem value={template.id} key={template.id}>
									<span className="overflow-hidden text-ellipsis block">
										{template.display_name ?? template.name}
									</span>
								</SelectItem>
							);
						})}
					</SelectContent>
				</Select>

				<Button size="sm" type="submit">
					<SendIcon />
					Run task
				</Button>
			</div>
		</form>
	);
};

type TasksTableProps = {
	templates: Template[];
};

const TasksTable: FC<TasksTableProps> = ({ templates }) => {
	const {
		data: tasks,
		error,
		refetch,
	} = useQuery({
		queryKey: ["tasks"],
		queryFn: () => data.fetchTasks(templates),
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
						<TableRow key={workspace.id}>
							<TableCell>
								<AvatarData
									title={
										<span className="block max-w-[520px] overflow-hidden text-ellipsis whitespace-nowrap">
											{prompt}
										</span>
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
								<WorkspaceAppStatus status={workspace.latest_app_status} />
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
							<TableCell className="pl-10">
								{agent && app && (
									<IconAppLink app={app} workspace={workspace} agent={agent} />
								)}
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
					<TableHead className="w-0" />
				</TableRow>
			</TableHeader>
			<TableBody>{body}</TableBody>
		</Table>
	);
};

type IconAppLinkProps = {
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
};

const IconAppLink: FC<IconAppLinkProps> = ({ app, workspace, agent }) => {
	const link = useAppLink(app, {
		workspace,
		agent,
	});

	return (
		<BaseIconLink
			key={app.id}
			label={`Open ${link.label}`}
			href={link.href}
			onClick={link.onClick}
		>
			<ExternalImage src={app.icon ?? "/icon/widgets.svg"} />
		</BaseIconLink>
	);
};

type BaseIconLinkProps = PropsWithChildren<{
	label: string;
	href: string;
	isLoading?: boolean;
	target?: string;
	onClick?: (e: React.MouseEvent<HTMLAnchorElement>) => void;
}>;

const BaseIconLink: FC<BaseIconLinkProps> = ({
	href,
	isLoading,
	label,
	children,
	target,
	onClick,
}) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button variant="outline" size="icon-lg" asChild>
						<a
							target={target}
							className={isLoading ? "animate-pulse" : ""}
							href={href}
							onClick={(e) => {
								e.stopPropagation();
								onClick?.(e);
							}}
						>
							{children}
							<span className="sr-only">{label}</span>
						</a>
					</Button>
				</TooltipTrigger>
				<TooltipContent>{label}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

type Task = {
	workspace: Workspace;
	prompt: string;
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
			return parameters[index].some((p) => p.name === "AI Prompt");
		});
	},

	// TODO: This function is inefficient because it fetches workspaces for each
	// template individually and its build parameters resulting in excessive API
	// calls and slow performance. Consider implementing a backend endpoint that
	// returns all AI-related workspaces in a single request to improve efficiency.
	async fetchTasks(aiTemplates: Template[]) {
		const workspaces = await Promise.all(
			aiTemplates.map((template) => {
				return API.getWorkspaces({
					q: `template:${template.name}`,
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
				const prompt = parameters.find((p) => p.name === "AI Prompt")?.value;

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
};

export default TasksPage;
