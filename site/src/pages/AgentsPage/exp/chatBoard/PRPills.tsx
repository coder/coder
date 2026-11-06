import { cn } from "cn";
import type { Chat, ChatDiffStatus } from "#/api/typesGenerated";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	getChatDisplayConfig,
	getPRIconConfig,
} from "../../components/ChatsSidebar/tree/statusConfig";
import {
	PRMenuLinks,
	prMenuContentClassName,
} from "../../components/PRMenuLinks";

/** Shape shared by every pill in a card's tag row: efforts and PRs. */
export const tagPillClassName =
	"relative z-[1] inline-flex h-5 shrink-0 cursor-pointer items-center gap-1 rounded border-0 bg-content-primary/5 px-1.5 text-xs text-content-secondary no-underline hover:text-content-primary";

/** The PRs a chat tracks that have a link, in the order the server lists them. */
export const chatPRs = (chat: Chat): readonly ChatDiffStatus[] =>
	getChatDisplayConfig(chat).prStatuses.filter((status) => status.url);

type PRState = "open" | "draft" | "merged" | "closed";

const prState = (status: ChatDiffStatus): PRState => {
	if (status.pull_request_state === "merged") return "merged";
	if (status.pull_request_state === "closed") return "closed";
	return status.pull_request_draft ? "draft" : "open";
};

// Work still in flight first.
const STATE_ORDER: readonly PRState[] = ["open", "draft", "merged", "closed"];

type PRPillsProps = { readonly prs: readonly ChatDiffStatus[] };

/**
 * One pill per PR state, each opening a menu of just that state's PRs, so a
 * mostly merged set does not read as open work and the open ones are one
 * click away.
 */
export const PRPills: React.FC<PRPillsProps> = ({ prs }) => {
	return STATE_ORDER.map((name) => {
		const matching = prs.filter((status) => prState(status) === name);
		const config = matching[0] && getPRIconConfig(matching[0]);
		if (!config) return null;
		return (
			<DropdownMenu key={name}>
				<DropdownMenuTrigger asChild>
					<button
						type="button"
						aria-label={`${matching.length} ${name} ${matching.length === 1 ? "pull request" : "pull requests"}`}
						className={tagPillClassName}
						onPointerDown={(e) => e.stopPropagation()}
					>
						<config.icon
							className={cn("size-3.5 shrink-0", config.className)}
						/>
						{matching.length}
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start" className={prMenuContentClassName}>
					<PRMenuLinks prStatuses={matching} Item={DropdownMenuItem} />
				</DropdownMenuContent>
			</DropdownMenu>
		);
	});
};
