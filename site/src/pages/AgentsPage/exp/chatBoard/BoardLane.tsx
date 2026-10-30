import { cn } from "cn";
import { BotIcon, ChevronRightIcon } from "lucide-react";
import { useRef } from "react";
import { Link } from "react-router";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ChatProjectIcon } from "../../components/ChatProjectIcon";
import { AssistantOpener } from "./Openers";

/** The project a lane files chats into, as far as the board knows it. */
export type LaneProject = Readonly<{
	id: string;
	organization_id: string;
	name: string;
}>;

type BoardLaneProps = {
	readonly name: string;
	/** Undefined for no project, or a project missing from the list. */
	readonly project: ChatProject | undefined;
	readonly cardCount: number;
	readonly collapsed: boolean;
	readonly onToggle: () => void;
	/** The lane's assistant chat, once one has been created. */
	readonly assistant: Chat | undefined;
	/** Opens the lane's assistant, creating it the first time. */
	readonly onAssistant: () => void;
	readonly onPreview: (chat: Chat, anchor: DOMRect) => void;
	readonly onPreviewEnd: () => void;
	readonly children: React.ReactNode;
};

/**
 * One project's own row of the board's columns, under a header that belongs
 * to the project: collapse, the project page, and its assistant.
 */
export const BoardLane: React.FC<BoardLaneProps> = ({
	name,
	project,
	cardCount,
	collapsed,
	onToggle,
	assistant,
	onAssistant,
	onPreview,
	onPreviewEnd,
	children,
}) => {
	const headerRef = useRef<HTMLElement>(null);
	const anchor = () =>
		headerRef.current?.getBoundingClientRect() ?? new DOMRect();
	return (
		<section
			aria-label={`${name} lane`}
			className="border-0 border-b border-solid border-border"
		>
			{/*
			  Sticky so it stays in view while the columns scroll sideways. The
			  chevron is centered on the column dots' axis and the name starts on
			  the column names' axis.
			*/}
			<header
				ref={headerRef}
				className={cn(
					"sticky left-0 flex w-fit items-center gap-1 pt-3 pr-4 pl-[22px] text-[13px] font-medium text-content-primary",
					// Expanded, the extra space sets the lane name apart from the column titles.
					collapsed ? "pb-3" : "pb-5",
				)}
			>
				<Button
					variant="subtle"
					size="icon"
					aria-label={collapsed ? `Expand ${name}` : `Collapse ${name}`}
					aria-expanded={!collapsed}
					className="size-4 min-w-0 rounded p-0 text-content-secondary [&>svg]:size-3.5! [&>svg]:p-0"
					onClick={onToggle}
				>
					<ChevronRightIcon
						className={cn("transition-transform", !collapsed && "rotate-90")}
					/>
				</Button>
				{project ? (
					<Link
						to={`/agents/projects/${project.id}`}
						className="flex items-center gap-1.5 text-content-primary no-underline hover:underline"
					>
						<ChatProjectIcon project={project} className="size-3.5" />
						{name}
					</Link>
				) : (
					<span className="text-content-secondary">{name}</span>
				)}
				<span className="ml-1 text-[11px] text-content-secondary/70 tabular-nums">
					{cardCount}
				</span>
				{/* An existing assistant gets the card assistant's opener: preview, pulse, unread. */}
				<span className="ml-1 flex">
					{assistant ? (
						<AssistantOpener
							assistant={assistant}
							onOpen={onAssistant}
							onPreview={(chat) => onPreview(chat, anchor())}
							onPreviewEnd={onPreviewEnd}
						/>
					) : (
						<Button
							variant="subtle"
							size="icon"
							aria-label={`Assistant for ${name}`}
							title={`Assistant for ${name}`}
							className="size-4 min-w-0 rounded p-0 text-content-secondary/60 [&>svg]:size-3.5! [&>svg]:p-0"
							onClick={onAssistant}
						>
							<BotIcon />
						</Button>
					)}
				</span>
			</header>
			{!collapsed && children}
		</section>
	);
};
