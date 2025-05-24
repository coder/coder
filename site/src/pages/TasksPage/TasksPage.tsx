import { API } from "api/api";
import { templates } from "api/queries/templates";
import { disabledRefetchOptions } from "api/queries/util";
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
import { CircleCheckIcon, ExternalLinkIcon, SendIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";

// This is how we identify AI templates
const AI_PROMPT_PARAMETER = "AI Prompt";

const TasksPage: FC = () => {
	return (
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

			<TaskForm />
			<Table className="mt-4">
				<TableHeader>
					<TableRow>
						<TableHead>Task</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>Created by</TableHead>
						<TableHead className="w-0" />
					</TableRow>
				</TableHeader>
				<TableBody>
					<TableRow>
						<TableCell>
							<AvatarData
								title={
									<span className="block max-w-[520px] overflow-hidden text-ellipsis whitespace-nowrap">
										Create template for Ruby on Rails with PostgresSQL and Redis
										using Docker for provisioning
									</span>
								}
								subtitle="Code Coder with Claude"
								avatar={
									<Avatar
										variant="icon"
										src="https://dev.coder.com/emojis/1f3c5.png"
										size="lg"
									/>
								}
							/>
						</TableCell>
						<TableCell>
							<div className="flex flex-col">
								<div className="flex items-center gap-2">
									<Spinner size="sm" loading />
									<span className="text-sm font-medium text-content-primary">
										Checking what is necessary to get started
									</span>
								</div>
								<span className="pl-[28px]">Working</span>
							</div>
						</TableCell>
						<TableCell>
							<AvatarData
								title="Bruno Quaresma"
								subtitle="2m ago"
								src="https://avatars.githubusercontent.com/u/3165839?v=4"
							/>
						</TableCell>
						<TableCell className="pl-10">
							<Button size="icon-lg" variant="outline">
								<ExternalImage src="https://uxwing.com/wp-content/themes/uxwing/download/brands-and-social-media/claude-ai-icon.png" />
							</Button>
						</TableCell>
					</TableRow>

					<TableRow>
						<TableCell>
							<AvatarData
								title="Update all npm dependencies on coder/site"
								subtitle="Code Coder with Claude"
								avatar={
									<Avatar
										variant="icon"
										src="https://dev.coder.com/emojis/1f3c5.png"
										size="lg"
									/>
								}
							/>
						</TableCell>
						<TableCell>
							<div className="flex flex-col">
								<div className="flex items-center gap-2">
									<CircleCheckIcon className="size-icon-sm text-content-success" />
									<span className="text-sm font-medium text-content-primary">
										All dependencies updated
									</span>
								</div>
								<span className="pl-[28px]">Success</span>
							</div>
						</TableCell>
						<TableCell>
							<AvatarData
								title="Bruno Quaresma"
								subtitle="10m ago"
								src="https://avatars.githubusercontent.com/u/3165839?v=4"
							/>
						</TableCell>
						<TableCell className="pl-10">
							<Button size="icon-lg" variant="outline">
								<ExternalImage src="https://uxwing.com/wp-content/themes/uxwing/download/brands-and-social-media/claude-ai-icon.png" />
							</Button>
						</TableCell>
					</TableRow>
				</TableBody>
			</Table>
		</Margins>
	);
};

export default TasksPage;

const TaskForm: FC = () => {
	const { data: templates, isLoading } = useQuery({
		queryKey: ["templates", "ai"],
		queryFn: fetchAITemplates,
		...disabledRefetchOptions,
	});

	return (
		<form className="border border-border border-solid rounded-lg p-4">
			<textarea
				name="prompt"
				placeholder="Write a task description..."
				className={`border-0 resize-none w-full h-full bg-transparent rounded-lg outline-none flex min-h-[60px]
						text-sm shadow-sm text-content-primary placeholder:text-content-secondary md:text-sm`}
			/>
			<div className="flex items-center justify-between">
				<Select name="templateID" disabled={isLoading}>
					<SelectTrigger className="w-52 text-xs [&_svg]:size-icon-xs border-0 bg-surface-secondary h-8 px-3">
						<SelectValue placeholder="Select a template" />
					</SelectTrigger>
					<SelectContent>
						{templates?.map((template) => {
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

				<Button size="sm" type="submit" disabled={isLoading}>
					<SendIcon />
					Run task
				</Button>
			</div>
		</form>
	);
};

async function fetchAITemplates() {
	const templates = await API.getTemplates();
	const parameters = await Promise.all(
		templates.map(async (template) =>
			API.getTemplateVersionRichParameters(template.active_version_id),
		),
	);
	return templates.filter((template, index) => {
		return parameters[index].some((p) => p.name === AI_PROMPT_PARAMETER);
	});
}
