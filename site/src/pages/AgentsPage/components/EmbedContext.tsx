import { API } from "api/api";
import { getAuthorizationKey } from "api/queries/authCheck";
import { meKey } from "api/queries/users";
import type { AuthorizationRequest } from "api/typesGenerated";
import { createContext, useContext } from "react";
import type { QueryClient } from "react-query";

interface EmbedContextValue {
	isEmbedded: boolean;
}

const EmbedContext = createContext<EmbedContextValue>({
	isEmbedded: false,
});

export const EmbedProvider = EmbedContext.Provider;

export const useEmbedContext = () => useContext(EmbedContext);

export const bootstrapChatEmbedSession = (
	authorization: AuthorizationRequest,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: async (token: string) =>
			bootstrapChatEmbedSessionFn({
				token,
				authorization,
				queryClient,
			}),
		onSuccess: (
			data: Awaited<ReturnType<typeof bootstrapChatEmbedSessionFn>>,
		) => {
			queryClient.setQueryData(meKey, data.user);
			queryClient.setQueryData(
				getAuthorizationKey(authorization),
				data.permissions,
			);
		},
	};
};

const bootstrapChatEmbedSessionFn = async ({
	token,
	authorization,
	queryClient,
}: {
	token: string;
	authorization: AuthorizationRequest;
	queryClient: QueryClient;
}) => {
	// This is the token forwarded by the VS Code extension's
	// ChatPanelProvider via the coder:vscode-auth-bootstrap
	// postMessage handshake. See chatPanelProvider.ts in
	// coder/vscode-coder for the sending side.
	API.setSessionToken(token);
	// Fetch user and permissions first, then set them in the cache
	// atomically. This avoids a race where invalidating the "me"
	// query causes isSignedIn to flip before permissions are ready.
	const [user, permissions] = await Promise.all([
		API.getAuthenticatedUser(),
		API.checkAuthorization(authorization),
	]);
	queryClient.setQueryData(meKey, user);
	queryClient.setQueryData(getAuthorizationKey(authorization), permissions);
	return {
		user,
		permissions,
	};
};
