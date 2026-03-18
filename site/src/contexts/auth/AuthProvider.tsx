import { isApiError } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import {
	hasFirstUser,
	login,
	logout,
	me,
	updateProfile as updateProfileOptions,
} from "api/queries/users";
import type { UpdateUserProfileRequest, User } from "api/typesGenerated";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type Permissions, permissionChecks } from "modules/permissions";
import {
	createContext,
	type FC,
	type PropsWithChildren,
	useCallback,
	useContext,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";

export type AuthContextValue = {
	isLoading: boolean;
	isSignedOut: boolean;
	isSigningOut: boolean;
	isConfiguringTheFirstUser: boolean;
	isSignedIn: boolean;
	isSigningIn: boolean;
	isUpdatingProfile: boolean;
	isError: boolean;
	user: User | undefined;
	permissions: Permissions | undefined;
	signInError: unknown;
	updateProfileError: unknown;
	signOut: () => void;
	signIn: (email: string, password: string) => Promise<void>;
	updateProfile: (data: UpdateUserProfileRequest) => void;
};

export const AuthContext = createContext<AuthContextValue | undefined>(
	undefined,
);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
	const { metadata } = useEmbeddedMetadata();
	const userMetadataState = metadata.user;

	const meOptions = me(userMetadataState);
	const userQuery = useQuery({
		...meOptions,
		retry: (failureCount, error) => {
			// Never retry on 401 — the user is simply not authenticated.
			if (isApiError(error) && error.response.status === 401) {
				return false;
			}
			return failureCount < 3;
		},
		retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 10000),
	});
	const hasFirstUserQuery = useQuery(hasFirstUser(userMetadataState));

	const permissionsQuery = useQuery({
		...checkAuthorization<Permissions>(
			{ checks: permissionChecks },
			metadata.permissions,
		),
		enabled: userQuery.data !== undefined,
		retry: (failureCount, error) => {
			if (isApiError(error) && error.response.status === 401) {
				return false;
			}
			return failureCount < 3;
		},
		retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 10000),
	});

	const queryClient = useQueryClient();
	const loginMutation = useMutation(
		login({ checks: permissionChecks }, queryClient),
	);

	const logoutMutation = useMutation(logout(queryClient));
	const updateProfileMutation = useMutation({
		...updateProfileOptions("me"),
		onSuccess: (user) => {
			queryClient.setQueryData(meOptions.queryKey, user);
		},
	});

	const isSignedOut =
		userQuery.isError &&
		isApiError(userQuery.error) &&
		userQuery.error.response.status === 401;
	const isSigningOut = logoutMutation.isPending;
	const isLoading =
		userQuery.isLoading ||
		hasFirstUserQuery.isLoading ||
		(userQuery.isSuccess && permissionsQuery.isLoading);
	const isConfiguringTheFirstUser =
		!hasFirstUserQuery.isLoading && !hasFirstUserQuery.data;
	const isSignedIn = userQuery.isSuccess && userQuery.data !== undefined;
	const isSigningIn = loginMutation.isPending;
	const isUpdatingProfile = updateProfileMutation.isPending;
	// Non-401 errors from the user query (e.g. network timeout, 500,
	// 502) represent a transient failure, not a sign-out. Exposing
	// this lets RequireAuth show a recoverable error screen instead
	// of crashing through to the error boundary.
	const isError =
		(userQuery.isError && !isSignedOut) ||
		(userQuery.isSuccess && permissionsQuery.isError);

	const signOut = useCallback(() => {
		logoutMutation.mutate();
	}, [logoutMutation]);

	const signIn = useCallback(
		async (email: string, password: string) => {
			await loginMutation.mutateAsync({ email, password });
		},
		[loginMutation],
	);

	const updateProfile = useCallback(
		(req: UpdateUserProfileRequest) => {
			const mutation = updateProfileMutation.mutateAsync(req);
			toast.promise(mutation, {
				loading: "Updating profile...",
				success: "Profile updated successfully.",
				error: "Failed to update profile.",
			});
		},
		[updateProfileMutation],
	);

	return (
		<AuthContext.Provider
			value={{
				isLoading,
				isSignedOut,
				isSigningOut,
				isConfiguringTheFirstUser,
				isSignedIn,
				isSigningIn,
				isUpdatingProfile,
				isError,
				signOut,
				signIn,
				updateProfile,
				user: userQuery.data,
				permissions: permissionsQuery.data as Permissions | undefined,
				signInError: loginMutation.error,
				updateProfileError: updateProfileMutation.error,
			}}
		>
			{children}
		</AuthContext.Provider>
	);
};

export const useAuthContext = () => {
	const context = useContext(AuthContext);

	if (!context) {
		throw new Error("useAuth should be used inside of <AuthProvider />");
	}

	return context;
};
