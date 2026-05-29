import { type FC, useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "react-query";
import {
	deleteMCPServerUserHeaderValues,
	mcpServerConfigs,
	mcpServerConfigsKey,
	mcpServerUserHeaderValues,
	updateMCPServerUserHeaderValues,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentSettingsUserMCPServersPageView,
	filterUserConfigurableServers,
} from "./AgentSettingsUserMCPServersPageView";

const AgentSettingsUserMCPServersPage: FC = () => {
	const queryClient = useQueryClient();

	const serversQuery = useQuery(mcpServerConfigs());

	// Only servers with custom_headers that require user-supplied values need
	// the per-user header status fetch. OAuth2 status comes from the existing
	// `auth_connected` field on the server config.
	const visibleServers = useMemo(
		() => filterUserConfigurableServers(serversQuery.data ?? []),
		[serversQuery.data],
	);

	const customHeaderServers = useMemo(
		() => visibleServers.filter((s) => s.auth_type === "custom_headers"),
		[visibleServers],
	);

	const headerQueries = useQueries({
		queries: customHeaderServers.map((server) => ({
			...mcpServerUserHeaderValues(server.id),
		})),
	});

	const headerValueStatus = useMemo(() => {
		const out: Record<string, Record<string, boolean>> = {};
		customHeaderServers.forEach((server, index) => {
			const data = headerQueries[index]?.data;
			if (data) {
				out[server.id] = data.has_values ?? {};
			}
		});
		return out;
	}, [customHeaderServers, headerQueries]);

	const loadingHeaderStatusIds = useMemo(() => {
		const ids = new Set<string>();
		customHeaderServers.forEach((server, index) => {
			if (headerQueries[index]?.isLoading) {
				ids.add(server.id);
			}
		});
		return ids;
	}, [customHeaderServers, headerQueries]);

	const updateMutation = useMutation(
		updateMCPServerUserHeaderValues(queryClient),
	);
	const deleteMutation = useMutation(
		deleteMCPServerUserHeaderValues(queryClient),
	);

	const onSaveHeaderValues = useCallback(
		async (
			server: TypesGen.MCPServerConfig,
			values: Record<string, string>,
		) => {
			await updateMutation.mutateAsync({ id: server.id, req: { values } });
		},
		[updateMutation],
	);

	const onClearHeaderValues = useCallback(
		async (server: TypesGen.MCPServerConfig) => {
			await deleteMutation.mutateAsync(server.id);
		},
		[deleteMutation],
	);

	// Reset the inline error state from the previous configure-dialog
	// session so the next user does not see a stale failure banner.
	const onResetSaveHeaderValuesError = useCallback(() => {
		updateMutation.reset();
		deleteMutation.reset();
	}, [updateMutation, deleteMutation]);

	// OAuth2 connect: open the experimental MCP OAuth2 connect URL in a popup
	// and refresh the server list when the popup posts back.
	const [oauth2PopupRef, setOAuth2PopupRef] = useState<Window | null>(null);

	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (event.origin !== location.origin) return;
			if (
				event.data?.type === "mcp-oauth2-complete" &&
				typeof event.data.serverID === "string"
			) {
				setOAuth2PopupRef(null);
				void queryClient.invalidateQueries({ queryKey: mcpServerConfigsKey });
			}
		};
		window.addEventListener("message", handler);
		return () => window.removeEventListener("message", handler);
	}, [queryClient]);

	useEffect(() => {
		if (!oauth2PopupRef) return;
		const interval = setInterval(() => {
			if (oauth2PopupRef.closed) {
				setOAuth2PopupRef(null);
			}
		}, 500);
		return () => clearInterval(interval);
	}, [oauth2PopupRef]);

	const onConnectOAuth2 = useCallback((server: TypesGen.MCPServerConfig) => {
		const connectUrl = `/api/experimental/mcp/servers/${encodeURIComponent(server.id)}/oauth2/connect`;
		const popup = window.open(connectUrl, "_blank", "width=900,height=600");
		if (popup) {
			setOAuth2PopupRef(popup);
		}
	}, []);

	return (
		<AgentSettingsUserMCPServersPageView
			servers={serversQuery.data}
			isLoadingServers={serversQuery.isLoading}
			serversError={serversQuery.error}
			headerValueStatus={headerValueStatus}
			loadingHeaderStatusIds={loadingHeaderStatusIds}
			onConnectOAuth2={onConnectOAuth2}
			onSaveHeaderValues={onSaveHeaderValues}
			onClearHeaderValues={onClearHeaderValues}
			isSavingHeaderValues={updateMutation.isPending}
			isClearingHeaderValues={deleteMutation.isPending}
			saveHeaderValuesError={updateMutation.error ?? deleteMutation.error}
			onResetSaveHeaderValuesError={onResetSaveHeaderValuesError}
		/>
	);
};

export default AgentSettingsUserMCPServersPage;
