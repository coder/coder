import type { FC } from "react";
import { useProxy } from "#/contexts/ProxyContext";
import { WorkspaceProxyView } from "./WorkspaceProxyView";

const WorkspaceProxyPage: FC = () => {
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
