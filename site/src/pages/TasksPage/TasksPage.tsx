import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { SelectOption } from "components/Combobox/Combobox.stories";
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
	SelectGroup,
	SelectItem,
	SelectLabel,
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
import { Textarea } from "components/Textarea/Textarea";
import { CircleCheckIcon, ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";

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

			<div className="border border-border border-solid rounded-lg p-4">
				<textarea
					placeholder="Write a task description..."
					className="resize-none w-full h-full bg-transparent rounded-lg block border-0 outline-none"
					rows={5}
				/>
				<Select value="apple">
					<SelectTrigger className="w-52 text-xs [&_svg]:size-icon-xs border-0 bg-surface-secondary h-8 px-3">
						<SelectValue placeholder="Select a template" />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="apple">
							<span className="overflow-hidden text-ellipsis block">
								Code Coder with Claude
							</span>
						</SelectItem>
					</SelectContent>
				</Select>
			</div>

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
