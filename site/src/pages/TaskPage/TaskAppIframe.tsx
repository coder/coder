import type { WorkspaceApp } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Spinner } from "components/Spinner/Spinner";
import { EllipsisVertical, ExternalLinkIcon, HouseIcon } from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import type { Task } from "modules/tasks/tasks";
import { type FC, useRef } from "react";
import { Link as RouterLink } from "react-router-dom";
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

	const appHref = (): string => {
		try {
			const url = new URL(link.href, location.href);
			if (pathname) {
				url.pathname = pathname;
			}
			return url.toString();
		} catch (err) {
			console.warn(`Failed to parse URL ${link.href} for app ${app.id}`, err);
			return link.href;
		}
	};

	const frameRef = useRef<HTMLIFrameElement>(null);
	const frameSrc = appHref();

	return (
		<div className={cn([active ? "flex" : "hidden", "w-full h-full flex-col"])}>
			{app.slug === "preview" && (
				<div className="bg-surface-tertiary flex items-center p-2 py-1 gap-1">
					<Button
						size="icon"
						variant="subtle"
						onClick={(e) => {
							e.preventDefault();
							if (frameRef.current?.contentWindow) {
								frameRef.current.contentWindow.location.href = appHref();
							}
						}}
					>
						<HouseIcon />
						<span className="sr-only">Home</span>
					</Button>

					{/* Possibly we will put a URL bar here, but for now we cannot due to
					 * cross-origin restrictions in iframes. */}
					<div className="w-full"></div>

					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button size="icon" variant="subtle" aria-label="More options">
								<EllipsisVertical aria-hidden="true" />
								<span className="sr-only">More options</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem asChild>
								<RouterLink to={frameSrc} target="_blank">
									<ExternalLinkIcon />
									Open app in new tab
								</RouterLink>
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
			)}

			{app.health === "healthy" ||
			app.health === "disabled" ||
			app.health === "unhealthy" ? (
				<iframe
					ref={frameRef}
					src={frameSrc}
					title={link.label}
					loading="eager"
					className={"w-full h-full border-0"}
					allow="clipboard-read; clipboard-write"
				/>
			) : app.health === "initializing" ? (
				<div className="w-full h-full flex items-center justify-center">
					<Spinner loading />
				</div>
			) : (
				<div className="w-full h-full flex flex-col items-center justify-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Error
					</h3>
					<span className="text-content-secondary text-sm">
						The app is in an unknown health state.
					</span>
				</div>
			)}
		</div>
	);
};
