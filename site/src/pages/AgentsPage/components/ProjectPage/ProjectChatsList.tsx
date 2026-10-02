import { useInfiniteQuery } from "react-query";
import { useOutletContext } from "react-router";
import { projectChats } from "#/api/queries/chats";
import type { ChatProject } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { ProjectChatsListView } from "./ProjectChatsListView";

type ProjectChatsListProps = {
	readonly project: ChatProject;
};

/** Loads a project's chats and wires their row actions to the agents page. */
export const ProjectChatsList: React.FC<ProjectChatsListProps> = ({
	project,
}) => {
	const { user } = useAuthenticated();
	const showCost = Boolean(useFeatureVisibility().aibridge);
	// Absent when the page renders outside the agents layout, which hides the
	// row actions.
	const outletContext = useOutletContext<AgentsPageOutletContext | undefined>();
	const chatsQuery = useInfiniteQuery(projectChats(project.id));
	// Watch events update cached rows in place, so a chat archived after the
	// page loads stays in the cache until the next refetch.
	const chats = chatsQuery.data?.pages.flat().filter((chat) => !chat.archived);

	return (
		<ProjectChatsListView
			chats={chats}
			error={chatsQuery.error}
			onRetry={() => void chatsQuery.refetch()}
			hasNextPage={chatsQuery.hasNextPage}
			isFetchingNextPage={chatsQuery.isFetchingNextPage}
			onLoadMore={() => void chatsQuery.fetchNextPage()}
			currentUser={user}
			actions={outletContext}
			showCost={showCost}
		/>
	);
};
