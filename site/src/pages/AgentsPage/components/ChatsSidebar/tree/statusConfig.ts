import type { LucideIcon } from "lucide-react";
import {
	AlertTriangleIcon,
	CheckIcon,
	GitMergeIcon,
	GitPullRequestArrowIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	Loader2Icon,
	PauseIcon,
} from "lucide-react";
import type { Chat, ChatDiffStatus, ChatStatus } from "#/api/typesGenerated";

type ChatIconConfig = {
	icon: LucideIcon;
	className: string;
};

const statusConfig = {
	waiting: { icon: CheckIcon, className: "text-content-secondary" },
	running: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	interrupting: { icon: PauseIcon, className: "text-content-warning" },
	requires_action: { icon: PauseIcon, className: "text-content-warning" },
	error: { icon: AlertTriangleIcon, className: "text-content-destructive" },
} as const;

const getStatusConfig = (status: ChatStatus): ChatIconConfig => {
	return statusConfig[status] ?? statusConfig.waiting;
};

const getPRIconConfig = (
	diffStatus: ChatDiffStatus | undefined,
): ChatIconConfig | undefined => {
	const state = diffStatus?.pull_request_state;
	if (!state) {
		return undefined;
	}
	if (state === "merged") {
		return { icon: GitMergeIcon, className: "text-git-merged-bright" };
	}
	if (state === "closed") {
		return {
			icon: GitPullRequestClosedIcon,
			className: "text-git-deleted-bright",
		};
	}
	if (diffStatus?.pull_request_draft) {
		return {
			icon: GitPullRequestDraftIcon,
			className: "text-content-secondary",
		};
	}
	return { icon: GitPullRequestArrowIcon, className: "text-git-added-bright" };
};

const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return chat.diff_status;
};

/**
 * Returns the icon and styling that represents a chat's current state.
 *
 * Combines `getStatusConfig` and `getPRIconConfig`: when the chat is in the
 * settled `waiting` state and has a linked PR, the PR icon takes precedence
 * so list rows surface the merge / closed / draft state instead of the
 * generic status icon.
 */
export const getChatDisplayConfig = (
	chat: Chat,
): {
	icon: LucideIcon;
	className: string;
	diffStatus: ChatDiffStatus | undefined;
} => {
	const diffStatus = getChatDiffStatus(chat);
	const baseConfig = getStatusConfig(chat.status);
	const prConfig =
		chat.status === "waiting" ? getPRIconConfig(diffStatus) : undefined;
	const config = prConfig ?? baseConfig;
	return {
		icon: config.icon,
		className: config.className,
		diffStatus,
	};
};
