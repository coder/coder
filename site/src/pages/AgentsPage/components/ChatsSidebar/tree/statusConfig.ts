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
	pending: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	running: { icon: Loader2Icon, className: "text-content-link animate-spin" },
	paused: { icon: PauseIcon, className: "text-content-warning" },
	requires_action: { icon: PauseIcon, className: "text-content-warning" },
	error: { icon: AlertTriangleIcon, className: "text-content-destructive" },
	completed: { icon: CheckIcon, className: "text-content-secondary" },
} as const;

export const getStatusConfig = (status: ChatStatus): ChatIconConfig => {
	return statusConfig[status] ?? statusConfig.completed;
};

export const getPRIconConfig = (
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

export const getChatDiffStatus = (chat: Chat): ChatDiffStatus | undefined => {
	return chat.diff_status;
};
