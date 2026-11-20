import { API } from "#/api/api";
import type { PreviewSignInRequest } from "#/api/typesGenerated";

/**
 * Signs in to a preview for one frame. The sign-in URL is short-lived, so each
 * frame passes its own `frameId` and the result is never reused or refetched.
 */
export const previewSignIn = (
	request: PreviewSignInRequest,
	frameId: string,
) => ({
	queryKey: ["previews", "sign-in", request, frameId],
	queryFn: () => API.experimental.signInToPreview(request),
	gcTime: 0,
	staleTime: Number.POSITIVE_INFINITY,
});
