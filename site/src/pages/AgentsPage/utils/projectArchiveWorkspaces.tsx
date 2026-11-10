import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import { getErrorMessage, isWorkspaceNotFound } from "#/api/errors";
import { chatProjectsKey } from "#/api/queries/chatProjects";
import { invalidateChatsByWorkspace } from "#/api/queries/chats";
import { workspaceQuotaKey } from "#/api/queries/workspaceQuota";
import {
	invalidateWorkspaceListQueries,
	workspaceByIdKey,
} from "#/api/queries/workspaces";
import type { Chat, ChatProject, Workspace } from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { toast } from "#/components/Toaster/toast";
import {
	fetchArchiveAndDeleteAction,
	notifyDeleteQueueState,
} from "./agentWorkspaceUtils";

/** A workspace behind one of the acting user's chats in a project. */
export type ProjectChatWorkspace = {
	readonly workspace: Workspace;
	/** True when the workspace existed before the chats that use it. */
	readonly existedBeforeChat: boolean;
};

export type ProjectWorkspaceAction = "leave" | "stop" | "delete";

const chatPageLimit = 50;
// GET /chats/by-workspace accepts at most this many workspace IDs.
const chatsByWorkspaceLimit = 25;

const listOwnProjectChats = async (projectId: string, userId: string) => {
	const chats: Chat[] = [];
	for (let offset = 0; ; offset += chatPageLimit) {
		const page = await API.experimental.getChats({
			limit: chatPageLimit,
			offset,
			q: "archived:false source:created_by_me",
			project_id: projectId,
		});
		chats.push(...page);
		if (page.length < chatPageLimit) {
			return chats.filter((chat) => chat.owner_id === userId);
		}
	}
};

export const projectChatWorkspacesKey = (projectId: string) =>
	[...chatProjectsKey, projectId, "own-chat-workspaces"] as const;

/**
 * Lists the workspaces behind the user's own unarchived chats in a project,
 * by name. Workspaces that no longer exist are left out.
 */
export const projectChatWorkspaces = (
	queryClient: QueryClient,
	project: ChatProject,
	userId: string,
) =>
	queryOptions({
		queryKey: projectChatWorkspacesKey(project.id),
		queryFn: async (): Promise<ProjectChatWorkspace[]> => {
			const chats = await listOwnProjectChats(project.id, userId);
			// A workspace counts as created by a chat when it appeared after the
			// first chat that uses it.
			const firstChatCreatedAt = new Map<string, string>();
			for (const chat of chats.flatMap((root) => [
				root,
				...(root.children ?? []),
			])) {
				if (!chat.workspace_id) {
					continue;
				}
				const current = firstChatCreatedAt.get(chat.workspace_id);
				if (!current || new Date(chat.created_at) < new Date(current)) {
					firstChatCreatedAt.set(chat.workspace_id, chat.created_at);
				}
			}
			const results = await Promise.all(
				[...firstChatCreatedAt].map(async ([workspaceId, chatCreatedAt]) => {
					const action = await fetchArchiveAndDeleteAction(
						queryClient,
						workspaceId,
						chatCreatedAt,
					);
					const workspace = queryClient.getQueryData<Workspace>(
						workspaceByIdKey(workspaceId),
					);
					if (action === "archive-only" || !workspace) {
						return [];
					}
					return [{ workspace, existedBeforeChat: action === "confirm" }];
				}),
			);
			return results
				.flat()
				.sort((a, b) => a.workspace.name.localeCompare(b.workspace.name));
		},
	});

const findWorkspacesInUse = async (workspaceIds: readonly string[]) => {
	const inUse = new Set<string>();
	for (let i = 0; i < workspaceIds.length; i += chatsByWorkspaceLimit) {
		const chatByWorkspace = await API.experimental.getChatsByWorkspace(
			workspaceIds.slice(i, i + chatsByWorkspaceLimit),
		);
		for (const workspaceId of Object.keys(chatByWorkspace)) {
			inUse.add(workspaceId);
		}
	}
	return inUse;
};

const isRunning = (workspace: Workspace) =>
	workspace.latest_build.transition === "start" &&
	workspace.latest_build.status === "running";

const workspacePath = (workspace: Workspace) =>
	`/@${workspace.owner_name}/${workspace.name}`;

type WorkspaceActionFailure = {
	readonly workspace: Workspace;
	readonly error: unknown;
};

const notifyWorkspaceActionResult = (
	action: "stop" | "delete",
	failed: readonly WorkspaceActionFailure[],
	skipped: readonly Workspace[],
	onOpenWorkspace: (path: string) => void,
) => {
	if (failed.length === 0 && skipped.length === 0) {
		return;
	}
	const workspaceLink = (workspace: Workspace) => (
		<Link
			href={workspacePath(workspace)}
			showExternalIcon={false}
			onClick={(event) => {
				event.preventDefault();
				onOpenWorkspace(workspacePath(workspace));
			}}
		>
			{workspace.name}
		</Link>
	);
	const description = (
		<ul className="m-0 flex list-none flex-col gap-1 p-0">
			{failed.map(({ workspace, error }) => (
				<li key={workspace.id}>
					{workspaceLink(workspace)}:{" "}
					{getErrorMessage(error, "Request failed.")}
				</li>
			))}
			{skipped.map((workspace) => (
				<li key={workspace.id}>
					{workspaceLink(workspace)}: still used by an unarchived chat.
				</li>
			))}
		</ul>
	);
	const count = (n: number) => `${n} ${n === 1 ? "workspace" : "workspaces"}`;
	if (failed.length > 0) {
		toast.error(
			`The project was archived, but ${count(failed.length)} could not be ${action === "stop" ? "stopped" : "deleted"}.`,
			{ description },
		);
		return;
	}
	toast.warning(
		`The project was archived. ${count(skipped.length)} still in use ${skipped.length === 1 ? "was" : "were"} skipped.`,
		{ description },
	);
};

/**
 * Stops or deletes the workspaces behind the user's chats after their project
 * is archived. Workspaces another unarchived chat still uses are skipped, and
 * one toast reports what was skipped or failed.
 */
export const runProjectWorkspaceAction = async (
	queryClient: QueryClient,
	action: "stop" | "delete",
	workspaces: readonly Workspace[],
	onOpenWorkspace: (path: string) => void,
) => {
	if (workspaces.length === 0) {
		return;
	}
	let inUse: Set<string>;
	try {
		inUse = await findWorkspacesInUse(workspaces.map(({ id }) => id));
	} catch (error) {
		toast.error(
			getErrorMessage(
				error,
				"Failed to check whether your workspaces are still in use.",
			),
			{
				description:
					"The project was archived, and its workspaces were left as they were.",
			},
		);
		return;
	}
	const skipped = workspaces.filter(({ id }) => inUse.has(id));
	const targets = workspaces.filter(({ id }) => !inUse.has(id));
	const failed: WorkspaceActionFailure[] = [];
	await Promise.all(
		targets.map(async (workspace) => {
			try {
				if (action === "delete") {
					notifyDeleteQueueState(
						workspace,
						await API.deleteWorkspace(workspace.id),
					);
				} else if (isRunning(workspace)) {
					await API.stopWorkspace(workspace.id);
				}
			} catch (error) {
				if (!isWorkspaceNotFound(error)) {
					failed.push({ workspace, error });
				}
			}
		}),
	);
	void invalidateWorkspaceListQueries(queryClient);
	void invalidateChatsByWorkspace(queryClient);
	for (const workspace of targets) {
		void queryClient.invalidateQueries({
			queryKey: workspaceByIdKey(workspace.id),
		});
	}
	if (action === "delete") {
		void queryClient.invalidateQueries({ queryKey: workspaceQuotaKey });
	}
	notifyWorkspaceActionResult(action, failed, skipped, onOpenWorkspace);
};
