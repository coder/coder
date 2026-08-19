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
	label: string;
};

const statusConfig = {
	waiting: {
		icon: CheckIcon,
		className: "text-content-secondary",
		label: "Idle",
	},
	running: {
		icon: Loader2Icon,
		className: "text-content-link animate-spin",
		label: "Working",
	},
	interrupting: {
		icon: PauseIcon,
		className: "text-content-warning",
		label: "Interrupting",
	},
	requires_action: {
		icon: PauseIcon,
		className: "text-content-warning",
		label: "Requires action",
	},
	error: {
		icon: AlertTriangleIcon,
		className: "text-content-destructive",
		label: "Error",
	},
} as const satisfies Record<ChatStatus, ChatIconConfig>;

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
		return {
			icon: GitMergeIcon,
			className: "text-git-merged-bright",
			label: "Pull request merged",
		};
	}
	if (state === "closed") {
		return {
			icon: GitPullRequestClosedIcon,
			className: "text-git-deleted-bright",
			label: "Pull request closed",
		};
	}
	if (diffStatus?.pull_request_draft) {
		return {
			icon: GitPullRequestDraftIcon,
			className: "text-content-secondary",
			label: "Draft pull request",
		};
	}
	return {
		icon: GitPullRequestArrowIcon,
		className: "text-git-added-bright",
		label: "Pull request open",
	};
};

const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return chat.diff_status;
};

/**
 * Returns the icons and styling that represent a chat's current state.
 *
 * The status icon always reflects the chat status. Any linked pull
 * request is surfaced separately via `prIcon` so rows can render it
 * next to the diff stats.
 */
export const getChatDisplayConfig = (
	chat: Chat,
): {
	icon: LucideIcon;
	className: string;
	label: string;
	prIcon: ChatIconConfig | undefined;
	diffStatus: ChatDiffStatus | undefined;
} => {
	const diffStatus = getChatDiffStatus(chat);
	const config = getStatusConfig(chat.status);
	return {
		icon: config.icon,
		className: config.className,
		label: config.label,
		prIcon: getPRIconConfig(diffStatus),
		diffStatus,
	};
};
