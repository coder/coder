import { cn } from "cn";
import { useId } from "react";
import type { Chat, User } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { getOwnPinnedChats } from "../../utils/pinnedChats";
import { LoadMoreSentinel } from "../ChatsSidebar/chats/LoadMoreSentinel";
import { ProjectChatRow, type ProjectChatRowActions } from "./ProjectChatRow";

type ProjectChatsListViewProps = {
	/** `undefined` until the first page loads. */
	readonly chats: readonly Chat[] | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
	readonly hasNextPage: boolean;
	readonly isFetchingNextPage: boolean;
	readonly onLoadMore: () => void;
	readonly currentUser: User;
	/** Omit to hide the row actions menus. */
	readonly actions?: ProjectChatRowActions;
	/** Fetches and shows each chat's AI Gateway cost. */
	readonly showCost: boolean;
};

type ProjectChatRowsProps = Pick<
	ProjectChatsListViewProps,
	"currentUser" | "actions" | "showCost"
> & {
	readonly chats: readonly Chat[];
};

const ProjectChatRows: React.FC<ProjectChatRowsProps> = ({
	chats,
	currentUser,
	actions,
	showCost,
}) => (
	<ul className="m-0 flex list-none flex-col p-0">
		{chats.map((chat) => (
			<ProjectChatRow
				key={chat.id}
				chat={chat}
				currentUser={currentUser}
				actions={actions}
				showCost={showCost}
			/>
		))}
	</ul>
);

type ProjectChatsSectionProps = {
	readonly title: string;
	readonly className?: string;
	readonly children: React.ReactNode;
};

const ProjectChatsSection: React.FC<ProjectChatsSectionProps> = ({
	title,
	className,
	children,
}) => {
	const headingId = useId();
	return (
		<section
			aria-labelledby={headingId}
			className={cn("flex flex-col gap-3", className)}
		>
			<h2
				id={headingId}
				className="m-0 text-sm font-medium text-content-primary"
			>
				{title}
			</h2>
			{children}
		</section>
	);
};

/**
 * The chats in a project, loaded page by page as the list scrolls. The
 * viewer's pinned chats lead in their own section, as in the sidebar.
 */
export const ProjectChatsListView: React.FC<ProjectChatsListViewProps> = ({
	chats,
	error,
	onRetry,
	hasNextPage,
	isFetchingNextPage,
	onLoadMore,
	currentUser,
	actions,
	showCost,
}) => {
	const pinnedChats = getOwnPinnedChats(chats ?? [], currentUser.id);
	const pinnedChatIds = new Set(pinnedChats.map((chat) => chat.id));
	const unpinnedChats = (chats ?? []).filter(
		(chat) => !pinnedChatIds.has(chat.id),
	);
	const isShowingChatsSection =
		pinnedChats.length === 0 ||
		unpinnedChats.length > 0 ||
		hasNextPage ||
		Boolean(error);
	const rowProps = { currentUser, actions, showCost };
	const errorAlert = Boolean(error) && (
		<ErrorAlert
			error={error}
			actions={
				<Button size="sm" variant="outline" onClick={onRetry}>
					Retry
				</Button>
			}
		/>
	);

	return (
		<div className="flex flex-col gap-8">
			{pinnedChats.length > 0 && (
				<ProjectChatsSection title="Pinned">
					<ProjectChatRows chats={pinnedChats} {...rowProps} />
				</ProjectChatsSection>
			)}
			{isShowingChatsSection && (
				<ProjectChatsSection title="Chats" className="flex-1">
					{chats === undefined ? (
						errorAlert || (
							<div className="flex flex-col">
								{[0, 1, 2].map((index) => (
									<div key={index} className="flex items-center gap-3 py-3">
										<Skeleton
											className="size-3.5 shrink-0"
											variant="circular"
										/>
										<Skeleton variant="text" className="w-1/3" />
									</div>
								))}
							</div>
						)
					) : chats.length === 0 ? (
						<>
							{/* Fills the grid row so the box matches the details panel
							    beside it. */}
							<div className="flex flex-1 items-center justify-center rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-content-secondary">
								No chats yet. Every chat will leverage the same settings and
								history.
							</div>
							{errorAlert}
						</>
					) : (
						<>
							{unpinnedChats.length > 0 && (
								<ProjectChatRows chats={unpinnedChats} {...rowProps} />
							)}
							{errorAlert}
							{hasNextPage && !error && (
								<LoadMoreSentinel
									onLoadMore={onLoadMore}
									isFetchingNextPage={isFetchingNextPage}
								/>
							)}
						</>
					)}
				</ProjectChatsSection>
			)}
		</div>
	);
};
