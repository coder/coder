import type { ChatContext, ChatContextResource } from "#/api/typesGenerated";
import { getPathBasename, getPathDirname } from "./path";

export type ContextResourceItem = {
	resource: ChatContextResource;
	name: string;
	dir: string;
};

/** Keep rejected resources visible without counting them as usable context. */
export function getContextInventory(context: ChatContext | undefined) {
	const files: ContextResourceItem[] = [];
	const skills: ContextResourceItem[] = [];
	const configs: ContextResourceItem[] = [];
	const servers: ContextResourceItem[] = [];
	const issues: ContextResourceItem[] = [];
	for (const resource of context?.resources ?? []) {
		const source = resource.source.trim();
		const name =
			resource.kind === "skill"
				? resource.skill_name?.trim() || getPathBasename(source)
				: resource.kind === "mcp_server"
					? source
					: getPathBasename(source);
		const item = {
			resource,
			name: name || "Unknown resource",
			dir: getPathDirname(source),
		};
		if (resource.kind === "mcp_server") servers.push(item);
		else if (resource.status !== "ok" || !name) issues.push(item);
		else if (resource.kind === "instruction_file") files.push(item);
		else if (resource.kind === "skill") skills.push(item);
		else configs.push(item);
	}
	return {
		files,
		skills,
		configs,
		servers,
		issues,
		known: context?.resources != null,
		connectedServers: servers.filter(({ resource }) => resource.status === "ok")
			.length,
	};
}

/** Preserve first-seen order and directory identity across inventory updates. */
export function groupContextResources(items: readonly ContextResourceItem[]) {
	const groups = new Map<string, ContextResourceItem[]>();
	for (const item of items) {
		const group = groups.get(item.dir);
		if (group) group.push(item);
		else groups.set(item.dir, [item]);
	}
	return Array.from(groups, ([dir, items]) => ({ dir, items }));
}
