import { cn } from "cn";
import type { Chat, ChatDiffStatus } from "#/api/typesGenerated";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { useTime } from "#/hooks/useTime";
import { shortRelativeTime } from "#/utils/time";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";
import {
	getChatDisplayConfig,
	getPRIconConfig,
} from "../../components/ChatsSidebar/tree/statusConfig";
import {
	PRMenuLinks,
	prMenuContentClassName,
} from "../../components/PRMenuLinks";
import { prNumber } from "../../utils/pullRequest";

type ChatStatusLineProps = {
	readonly chat: Chat;
	/** Placement in the card grid; the line renders nothing when there is nothing to say. */
	readonly className?: string;
};

const AGE_REFRESH_MS = 60_000;

type AgeProps = { readonly date: string | number };

/**
 * The age of `date`, kept current while it stays on screen. A plain
 * shortRelativeTime call would freeze: the React Compiler caches it by
 * `date` and cannot see that it reads the clock.
 */
export const RelativeAge: React.FC<AgeProps> = ({ date }) => (
	// useTime keeps its first value when its input changes, so a new date
	// gets a new instance.
	<TickingAge key={date} date={date} />
);

const TickingAge: React.FC<AgeProps> = ({ date }) => {
	const age = useTime(() => shortRelativeTime(date), {
		interval: AGE_REFRESH_MS,
	});
	return age;
};

/**
 * One line under a chat's title: PR chip, last turn text, then the age at
 * the right edge. The age is the chat's last change (`updated_at`), which
 * board label writes also bump, as in the sidebar. While the chat works it
 * would read "now" and say nothing, so it is omitted until the chat
 * settles. Shared by single cards and group rows.
 */
export const ChatStatusLine: React.FC<ChatStatusLineProps> = ({
	chat,
	className,
}) => {
	const { prStatuses } = getChatDisplayConfig(chat);
	const settled = !isActiveChatStatus(chat.status);
	if (!chat.last_turn_summary && prStatuses.length === 0 && !settled) {
		return null;
	}
	return (
		<div
			className={cn(
				"flex min-w-0 items-center gap-x-1.5 text-xs leading-4 text-content-secondary",
				className,
			)}
		>
			<PRChip prStatuses={prStatuses} />
			{chat.last_turn_summary && (
				<span className="min-w-0 flex-1 truncate">
					{chat.last_turn_summary}
				</span>
			)}
			{settled && (
				// The ⋮ glyph above ends ~6px inside its button; the age lines up
				// with the glyph, not the box.
				<time
					dateTime={chat.updated_at}
					className="ml-auto shrink-0 pr-1.5 text-[11px] tabular-nums text-content-secondary/70"
				>
					<RelativeAge date={chat.updated_at} />
				</time>
			)}
		</div>
	);
};

const chipClassName =
	"relative z-[1] inline-flex h-4 shrink-0 cursor-pointer items-center gap-1 rounded border-0 bg-content-primary/5 px-1.5 font-mono text-[11px] text-content-secondary no-underline hover:text-content-primary";

type PRChipProps = { readonly prStatuses: readonly ChatDiffStatus[] };

/**
 * One PR links straight to it. Several open the same menu as the top bar,
 * and the chip counts them per state, so a mostly merged set does not read
 * as open work and the chip stays short however many PRs there are.
 */
const PRChip: React.FC<PRChipProps> = ({ prStatuses }) => {
	const [sole] = prStatuses;
	if (prStatuses.length === 1 && sole.url) {
		const number = prNumber(sole);
		const visible = number ? `#${number}` : "PR";
		const state = getPRIconConfig(sole);
		return (
			<a
				href={sole.url}
				target="_blank"
				rel="noreferrer"
				aria-label={state ? `${visible}, ${state.label}` : visible}
				className={chipClassName}
				onPointerDown={(e) => e.stopPropagation()}
			>
				{state && (
					<state.icon className={cn("size-3 shrink-0", state.className)} />
				)}
				{visible}
			</a>
		);
	}
	if (prStatuses.length < 2) return null;
	const counts = countByState(prStatuses);
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					aria-label={`${counts.map((c) => `${c.count} ${c.name}`).join(", ")} pull requests`}
					className={cn(chipClassName, "gap-1.5")}
					onPointerDown={(e) => e.stopPropagation()}
				>
					{counts.map(({ name, config, count }) => (
						<span key={name} className="inline-flex items-center gap-0.5">
							<config.icon
								className={cn("size-3 shrink-0", config.className)}
							/>
							{count}
						</span>
					))}
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className={prMenuContentClassName}>
				<PRMenuLinks prStatuses={prStatuses} Item={DropdownMenuItem} />
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

type PRState = "open" | "draft" | "merged" | "closed";

const prState = (status: ChatDiffStatus): PRState => {
	if (status.pull_request_state === "merged") return "merged";
	if (status.pull_request_state === "closed") return "closed";
	return status.pull_request_draft ? "draft" : "open";
};

// Work still in flight first.
const STATE_ORDER: readonly PRState[] = ["open", "draft", "merged", "closed"];

const countByState = (prStatuses: readonly ChatDiffStatus[]) =>
	STATE_ORDER.flatMap((name) => {
		const matching = prStatuses.filter((status) => prState(status) === name);
		const config = matching[0] && getPRIconConfig(matching[0]);
		return config ? [{ name, config, count: matching.length }] : [];
	});
