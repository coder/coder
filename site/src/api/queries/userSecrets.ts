import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const userSecretsKey = (userId: string) => ["users", userId, "secrets"];

export const userSecrets = (userId: string) => {
	return {
		queryKey: userSecretsKey(userId),
		queryFn: () => API.getUserSecrets(userId),
	};
};

export const createUserSecret = (queryClient: QueryClient, userId: string) => {
	return {
		mutationFn: (request: TypesGen.CreateUserSecretRequest) =>
			API.createUserSecret(userId, request),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: userSecretsKey(userId),
			});
		},
	};
};

export const updateUserSecret = (queryClient: QueryClient, userId: string) => {
	return {
		mutationFn: ({
			name,
			request,
		}: {
			name: string;
			request: TypesGen.UpdateUserSecretRequest;
		}) => API.updateUserSecret(userId, name, request),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: userSecretsKey(userId),
			});
		},
	};
};

export const deleteUserSecret = (queryClient: QueryClient, userId: string) => {
	return {
		mutationFn: (name: string) => API.deleteUserSecret(userId, name),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: userSecretsKey(userId),
			});
		},
	};
};
