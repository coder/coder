import { type FC, lazy, Suspense } from "react";
import { Navigate } from "react-router";
import { AgentChatPageSkeleton } from "../../components/AgentsSkeletons";
import { useChatBoardEnabled } from "./useChatBoardEnabled";

// Lazy here rather than in the router so a visit with the flag off redirects
// without downloading the board.
const ChatBoardPage = lazy(() => import("./ChatBoardPage"));

/** Route element for the board: renders it when enabled, else leaves. */
const ChatBoardRoute: FC = () => {
	const [enabled] = useChatBoardEnabled();
	if (!enabled) {
		return <Navigate to="/agents" replace />;
	}
	return (
		<Suspense fallback={<AgentChatPageSkeleton />}>
			<ChatBoardPage />
		</Suspense>
	);
};

export default ChatBoardRoute;
