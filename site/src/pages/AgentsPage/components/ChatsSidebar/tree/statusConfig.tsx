import type { LucideIcon } from "lucide-react";
import {
	AlertTriangleIcon,
	CheckIcon,
	GitMergeIcon,
	GitPullRequestArrowIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	PauseIcon,
} from "lucide-react";
import type { ComponentType, FC, SVGProps } from "react";
import type { Chat, ChatDiffStatus, ChatStatus } from "#/api/typesGenerated";
import { Spinner } from "#/components/Spinner/Spinner";

type ChatIcon = LucideIcon | ComponentType<{ className?: string }>;

type ChatIconConfig = {
	icon: ChatIcon;
	className: string;
};

/**
 * Binds the shared Spinner into the icon slot. The status icon stays
 * mounted for the lifetime of a long-running chat, so it needs the
 * cheap stepped Spinner rather than a smooth `animate-spin` icon.
 */
const RunningSpinner: FC<SVGProps<SVGSVGElement>> = (props) => (
	<Spinner loading {...props} />
);

const statusConfig = {
	waiting: { icon: CheckIcon, className: "text-content-secondary" },
	running: { icon: RunningSpinner, className: "text-content-link" },
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
	icon: ChatIcon;
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
