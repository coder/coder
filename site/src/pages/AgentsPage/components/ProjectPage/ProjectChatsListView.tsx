import { useId } from "react";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
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
	readonly currentUserId: string;
	/** Omit to hide the row actions menus. */
	readonly actions?: ProjectChatRowActions;
};

/** The chats in a project, loaded page by page as the list scrolls. */
export const ProjectChatsListView: React.FC<ProjectChatsListViewProps> = ({
	chats,
	error,
	onRetry,
	hasNextPage,
	isFetchingNextPage,
	onLoadMore,
	currentUserId,
	actions,
}) => {
	const headingId = useId();
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
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<h2
				id={headingId}
				className="m-0 text-sm font-medium text-content-primary"
			>
				{chats
					? `${chats.length}${hasNextPage ? "+" : ""} ${
							chats.length === 1 && !hasNextPage ? "Chat" : "Chats"
						}`
					: "Chats"}
			</h2>
			{chats === undefined ? (
				errorAlert || (
					<div className="flex flex-col divide-y divide-solid divide-border rounded-lg border border-solid border-border">
						{[0, 1, 2].map((index) => (
							<div key={index} className="flex items-center gap-3 px-4 py-3">
								<Skeleton className="size-3.5 shrink-0" variant="circular" />
								<Skeleton variant="text" className="w-1/3" />
							</div>
						))}
					</div>
				)
			) : chats.length === 0 ? (
				<>
					<div className="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-content-secondary">
						No chats yet. Every chat will leverage the same settings and
						history.
					</div>
					{errorAlert}
				</>
			) : (
				<>
					<ul className="m-0 flex list-none flex-col divide-y divide-solid divide-border overflow-hidden rounded-lg border border-solid border-border p-0">
						{chats.map((chat) => (
							<ProjectChatRow
								key={chat.id}
								chat={chat}
								currentUserId={currentUserId}
								actions={actions}
							/>
						))}
					</ul>
					{errorAlert}
					{hasNextPage && !error && (
						<LoadMoreSentinel
							onLoadMore={onLoadMore}
							isFetchingNextPage={isFetchingNextPage}
						/>
					)}
				</>
			)}
		</section>
	);
};
