import type { FC } from "react";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { WorkspaceProxyView } from "./WorkspaceProxyView";

const WorkspaceProxyPage: FC = () => {
	const { permissions } = useAuthenticated();
	const {
		proxyLatencies,
		proxies,
		error: proxiesError,
		isFetched: proxiesFetched,
		isLoading: proxiesLoading,
		proxy,
	} = useProxy();
	const { workspace_proxy: workspaceProxyEnabled } = useFeatureVisibility();

	return (
		<WorkspaceProxyView
			proxyLatencies={proxyLatencies}
			proxies={proxies}
			isLoading={proxiesLoading}
			hasLoaded={proxiesFetched}
			getWorkspaceProxiesError={proxiesError}
			preferredProxy={proxy.proxy}
			showPaywall={!workspaceProxyEnabled}
			permissions={permissions}
		/>
	);
};

export default WorkspaceProxyPage;
