import type { WorkspaceApp } from "api/typesGenerated";
import { useAppLink } from "modules/apps/useAppLink";
import type { Task } from "modules/tasks/tasks";
import type { FC } from "react";
import { cn } from "utils/cn";

type TaskAppIFrameProps = {
	task: Task;
	app: WorkspaceApp;
	active: boolean;
	pathname?: string;
};

export const TaskAppIFrame: FC<TaskAppIFrameProps> = ({
	task,
	app,
	active,
	pathname,
}) => {
	const agent = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a)
		.find((a) => a.apps.some((a) => a.id === app.id));

	if (!agent) {
		throw new Error(`Agent for app ${app.id} not found in task workspace`);
	}

	const link = useAppLink(app, {
		agent,
		workspace: task.workspace,
	});

	let href = link.href;
	try {
		const url = new URL(link.href);
		if (pathname) {
			url.pathname = pathname;
		}
		href = url.toString();
	} catch (err) {
		console.warn(`Failed to parse URL ${link.href} for app ${app.id}`, err);
	}

	return (
		<iframe
			src={href}
			title={link.label}
			loading="eager"
			className={cn([active ? "block" : "hidden", "w-full h-full border-0"])}
			allow="clipboard-read; clipboard-write"
		/>
	);
};
