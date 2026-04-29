import {
	createContext,
	type PropsWithChildren,
	useEffect,
	useState,
} from "react";
import { useQueryClient } from "react-query";
import { ChatSessionManager } from "./ChatSessionManager";
import type { ChatSessionManagerRuntimeDeps } from "./types";

type ChatSessionsProviderProps = PropsWithChildren<{
	setChatErrorReason: ChatSessionManagerRuntimeDeps["setChatErrorReason"];
	clearChatErrorReason: ChatSessionManagerRuntimeDeps["clearChatErrorReason"];
}>;

const ChatSessionsContext = createContext<ChatSessionManager | null>(null);

export const ChatSessionsProvider = ({
	children,
	setChatErrorReason,
	clearChatErrorReason,
}: ChatSessionsProviderProps) => {
	const queryClient = useQueryClient();
	const [manager] = useState(
		() =>
			new ChatSessionManager({
				queryClient,
				setChatErrorReason,
				clearChatErrorReason,
			}),
	);

	useEffect(() => {
		manager.updateRuntimeDeps({
			queryClient,
			setChatErrorReason,
			clearChatErrorReason,
		});
	}, [manager, queryClient, setChatErrorReason, clearChatErrorReason]);

	useEffect(() => {
		return () => {
			manager.dispose();
		};
	}, [manager]);

	return (
		<ChatSessionsContext.Provider value={manager}>
			{children}
		</ChatSessionsContext.Provider>
	);
};

export { ChatSessionsContext as InternalChatSessionsContext };
