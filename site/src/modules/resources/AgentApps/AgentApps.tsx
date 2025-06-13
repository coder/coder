import type { WorkspaceApp } from "api/typesGenerated";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Folder } from "lucide-react";
import type { FC } from "react";
import { AgentButton } from "../AgentButton";
import { AppLink } from "../AppLink/AppLink";

type AppsProps = {
	section: AppSection;
	agent: WorkspaceAgent;
	workspace: Workspace;
};

export const Apps: FC<AppsProps> = ({ section, agent, workspace }) => {
	return section.group ? (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<AgentButton>
					<Folder />
					{section.group}
				</AgentButton>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start">
				{section.apps.map((app) => (
					<DropdownMenuItem key={app.slug}>
						<AppLink grouped app={app} agent={agent} workspace={workspace} />
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	) : (
		<>
			{section.apps.map((app) => (
				<AppLink key={app.slug} app={app} agent={agent} workspace={workspace} />
			))}
		</>
	);
};

type AppSection = {
	/**
	 * If there is no `group`, just render all of the apps inline. If there is a
	 * group name, show them all in a dropdown.
	 */
	group?: string;

	apps: WorkspaceApp[];
};

/**
 * Groups apps by their `group` property. Apps with the same group are placed
 * in the same section. Apps without a group are placed in their own section.
 *
 * The algorithm assumes that apps are already sorted by group, meaning that
 * every ungrouped section is expected to have a group in between, to make the
 * algorithm a little simpler to implement.
 */
export function organizeAgentApps(apps: readonly WorkspaceApp[]): AppSection[] {
	let currentSection: AppSection | undefined = undefined;
	const appGroups: AppSection[] = [];
	const groupsByName = new Map<string, AppSection>();

	for (const app of apps) {
		if (app.hidden) {
			continue;
		}

		if (!currentSection || app.group !== currentSection.group) {
			const existingSection = groupsByName.get(app.group!);
			if (existingSection) {
				currentSection = existingSection;
			} else {
				currentSection = {
					group: app.group,
					apps: [],
				};
				appGroups.push(currentSection);
				if (app.group) {
					groupsByName.set(app.group, currentSection);
				}
			}
		}

		currentSection.apps.push(app);
	}

	return appGroups;
}
