import { paginatedSession } from "api/queries/aiBridge";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { AISessionPageView } from "./AISessionPageView";

const AISessionPage: FC = () => {
	const { sessionId = "" } = useParams<{ sessionId: string }>();

	const sessionQuery = useQuery({
		...paginatedSession(sessionId),
		enabled: !!sessionId,
	});

	return (
		<AISessionPageView
			sessionId={sessionId}
			session={sessionQuery.data}
			loading={sessionQuery.isLoading}
		/>
	);
};

export default AISessionPage;
