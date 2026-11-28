import type { QueryClient, UseMutationOptions } from "react-query";
import { API } from "#/api/api";
import type { ExternalAuth } from "#/api/typesGenerated";

export const externalAuthsKey = ["external-auth"];

// Returns all configured external auths for a given user.
export const externalAuths = () => {
	return {
		queryKey: externalAuthsKey,
		queryFn: () => API.getUserExternalAuthProviders(),
	};
};

export const externalAuthProvider = (providerId: string) => {
	return {
		queryKey: ["external-auth", providerId],
		queryFn: () => API.getExternalAuthProvider(providerId),
	};
};

export const externalAuthDevice = (providerId: string) => {
	return {
		queryFn: () => API.getExternalAuthDevice(providerId),
		queryKey: ["external-auth", providerId, "device"],
	};
};

export const exchangeExternalAuthDevice = (
	providerId: string,
	deviceCode: string,
) => {
	return {
		queryFn: () =>
			API.exchangeExternalAuthDevice(providerId, {
				device_code: deviceCode,
			}),
		queryKey: ["external-auth", providerId, "device", deviceCode],
	};
};

export const validateExternalAuth = (
	queryClient: QueryClient,
): UseMutationOptions<ExternalAuth, unknown, string> => {
	return {
		mutationFn: API.getExternalAuthProvider,
		onSuccess: (data, providerId) => {
			queryClient.setQueryData(["external-auth", providerId], data);
			// Only the list response carries validate_error, so refetch it to
			// replace an error from before this validation.
			void queryClient.invalidateQueries({
				queryKey: externalAuthsKey,
				exact: true,
			});
		},
	};
};

export const unlinkExternalAuths = (queryClient: QueryClient) => {
	return {
		mutationFn: API.unlinkExternalAuthProvider,
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: ["external-auth"],
			});
		},
	};
};
