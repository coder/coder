import { API } from "#/api/api";
import type { PreviewSignInRequest } from "#/api/typesGenerated";

/** Where a preview loads in the request's region. Fetching it mints no key. */
export const previewAddress = (request: PreviewSignInRequest) => ({
	queryKey: ["previews", "address", request],
	queryFn: () => API.experimental.getPreviewAddress(request),
});

/** Signs the browser in to a preview's host, which mints an API key. */
export const previewSignIn = () => ({
	mutationFn: (request: PreviewSignInRequest) =>
		API.experimental.signInToPreview(request),
});
