import { API } from "api/api";
import type { Region } from "api/typesGenerated";
import type { MetadataState } from "hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

const regionsKey = ["regions"] as const;

export const regions = (metadata: MetadataState<readonly Region[]>) => {
	return cachedQuery({
		metadata,
		queryKey: regionsKey,
		queryFn: () => API.getWorkspaceProxyRegions(),
	});
};
