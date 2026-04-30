import type { FC } from "react";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import { TerminalIcon } from "#/components/Icons/TerminalIcon";
import { VSCodeIcon } from "#/components/Icons/VSCodeIcon";
import { cn } from "#/utils/cn";
import { DisplayAppNameMap } from "./AppLink/AppLink";
import { AppPreview } from "./AppLink/AppPreview";
import { BaseIcon } from "./AppLink/BaseIcon";

interface AgentRowPreviewStyles {
	// Helpful when there are more than one row so the values are aligned
	// When it is only one row, it is better to have than "flex" and not hard aligned
	alignValues?: boolean;
}
interface AgentRowPreviewProps extends AgentRowPreviewStyles {
	agent: WorkspaceAgent;
}

export const AgentRowPreview: FC<AgentRowPreviewProps> = ({
	agent,
	alignValues,
}) => {
	return (
		<div
			key={agent.id}
			className="relative flex flex-row items-center justify-between gap-4 bg-surface-secondary px-8 pb-4 pt-4 text-base [&:not(:last-child)]:pb-0 after:absolute after:left-[43px] after:top-0 after:h-full after:w-0.5 after:bg-border after:content-['']"
		>
			<div className="flex flex-row items-baseline gap-4">
				<div className="flex w-6 shrink-0 justify-center">
					<div className="relative z-[1] size-2.5 rounded-full border-solid border-2 border-content-secondary bg-surface-secondary" />
				</div>
				<div className="flex flex-row items-baseline gap-8 font-normal text-sm text-content-secondary max-md:flex-wrap max-md:gap-4">
					<div
						className={cn(
							"flex shrink-0 flex-row items-baseline gap-2 max-md:w-fit max-md:flex-col max-md:items-start max-md:gap-2",
							alignValues && "sm:min-w-[240px]",
						)}
					>
						<span>Agent:</span>
						<span className="text-content-primary">{agent.name}</span>
					</div>

					<div
						className={cn(
							"flex shrink-0 flex-row items-baseline gap-2 max-md:w-fit max-md:flex-col max-md:items-start max-md:gap-2",
							alignValues && "sm:min-w-[100px]",
						)}
					>
						<span>OS:</span>
						<span className="font-normal text-sm capitalize text-content-primary">
							{agent.operating_system}
						</span>
					</div>

					<div className="flex flex-row items-center gap-2 max-md:w-fit max-md:flex-col max-md:items-start max-md:gap-2">
						<span>Apps:</span>
						<div className="flex flex-row items-center gap-1 flex-wrap">
							{/* We display all modules returned in agent.apps */}
							{agent.apps.map((app) => (
								<AppPreview key={app.slug}>
									<BaseIcon app={app} />
									{app.display_name}
								</AppPreview>
							))}
							{/* Additionally, we display any apps that are visible, e.g.
              apps that are included in agent.display_apps */}
							{agent.display_apps.includes("web_terminal") && (
								<AppPreview>
									<TerminalIcon className="size-3" />
									{DisplayAppNameMap.web_terminal}
								</AppPreview>
							)}
							{agent.display_apps.includes("ssh_helper") && (
								<AppPreview>{DisplayAppNameMap.ssh_helper}</AppPreview>
							)}
							{agent.display_apps.includes("port_forwarding_helper") && (
								<AppPreview>
									{DisplayAppNameMap.port_forwarding_helper}
								</AppPreview>
							)}
							{/* VSCode display apps (vscode, vscode_insiders) get special presentation */}
							{agent.display_apps.includes("vscode") ? (
								<AppPreview>
									<VSCodeIcon className="size-3" />
									{DisplayAppNameMap.vscode}
								</AppPreview>
							) : (
								agent.display_apps.includes("vscode_insiders") && (
									<AppPreview>
										<VSCodeIcon className="size-3" />
										{DisplayAppNameMap.vscode_insiders}
									</AppPreview>
								)
							)}
							{agent.apps.length === 0 && agent.display_apps.length === 0 && (
								<span className="text-content-primary">None</span>
							)}
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};
