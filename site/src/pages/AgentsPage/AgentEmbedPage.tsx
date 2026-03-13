import { Loader } from "components/Loader/Loader";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { ProxyProvider } from "contexts/ProxyContext";
import { DashboardProvider } from "modules/dashboard/DashboardProvider";
import { type FC, useCallback, useMemo, useState } from "react";
import { Outlet, useParams } from "react-router";
import type { AgentsOutletContext } from "./AgentsPage";
import { EmbedProvider } from "./EmbedContext";

const AgentEmbedPage: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	if (!agentId) {
		throw new Error("AgentEmbedPage requires an agentId route parameter.");
	}

	const auth = useAuthContext();
	const [chatErrorReasons, setChatErrorReasons] = useState<
		Record<string, string>
	>({});
	const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);

	const setChatErrorReason = useCallback((chatId: string, reason: string) => {
		const trimmedReason = reason.trim();
		if (!chatId || !trimmedReason) {
			return;
		}
		setChatErrorReasons((current) => {
			if (current[chatId] === trimmedReason) {
				return current;
			}
			return {
				...current,
				[chatId]: trimmedReason,
			};
		});
	}, []);

	const clearChatErrorReason = useCallback((chatId: string) => {
		if (!chatId) {
			return;
		}
		setChatErrorReasons((current) => {
			if (!(chatId in current)) {
				return current;
			}
			const next = { ...current };
			delete next[chatId];
			return next;
		});
	}, []);

	const requestArchiveAgent = useCallback((chatId: string) => {
		if (!chatId) {
			throw new Error("requestArchiveAgent requires a chatId.");
		}
	}, []);

	const requestUnarchiveAgent = useCallback((chatId: string) => {
		if (!chatId) {
			throw new Error("requestUnarchiveAgent requires a chatId.");
		}
	}, []);

	const requestArchiveAndDeleteWorkspace = useCallback(
		(chatId: string, workspaceId: string) => {
			if (!chatId) {
				throw new Error("requestArchiveAndDeleteWorkspace requires a chatId.");
			}
			if (!workspaceId) {
				throw new Error(
					"requestArchiveAndDeleteWorkspace requires a workspaceId.",
				);
			}
		},
		[],
	);

	const onToggleSidebarCollapsed = useCallback(() => {
		setIsSidebarCollapsed((current) => !current);
	}, []);

	const outletContext = useMemo<AgentsOutletContext>(
		() => ({
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
			requestUnarchiveAgent,
			requestArchiveAndDeleteWorkspace,
			isSidebarCollapsed,
			onToggleSidebarCollapsed,
		}),
		[
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
			requestUnarchiveAgent,
			requestArchiveAndDeleteWorkspace,
			isSidebarCollapsed,
			onToggleSidebarCollapsed,
		],
	);

	if (auth.isSignedIn) {
		return (
			<EmbedProvider value={{ isEmbedded: true }}>
				<DashboardProvider>
					<ProxyProvider>
						<Outlet context={outletContext} />
					</ProxyProvider>
				</DashboardProvider>
			</EmbedProvider>
		);
	}

	if (auth.isLoading) {
		return (
			<div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-surface-primary px-6 text-center">
				<Loader label="Loading" />
				<p className="max-w-md text-sm text-content-secondary">Loading…</p>
			</div>
		);
	}

	return (
		<div className="flex min-h-screen items-center justify-center bg-surface-primary px-6 text-center">
			<p className="max-w-md text-sm text-content-secondary">
				Not authenticated.
			</p>
		</div>
	);
};

export default AgentEmbedPage;
