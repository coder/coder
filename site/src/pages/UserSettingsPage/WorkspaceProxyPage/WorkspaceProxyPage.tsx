import { useProxy } from "contexts/ProxyContext";
import type { FC } from "react";
import { WorkspaceProxyView } from "./WorkspaceProxyView";

export const WorkspaceProxyPage: FC = () => {
	const {
		proxyLatencies,
		proxies,
		error: proxiesError,
		isFetched: proxiesFetched,
		isLoading: proxiesLoading,
		proxy,
	} = useProxy();

	return (
		<WorkspaceProxyView
			proxyLatencies={proxyLatencies}
			proxies={proxies}
			isLoading={proxiesLoading}
			hasLoaded={proxiesFetched}
			getWorkspaceProxiesError={proxiesError}
			preferredProxy={proxy.proxy}
		/>
	);
};

export default WorkspaceProxyPage;
