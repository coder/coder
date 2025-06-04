import {
	type AuthContextValue,
	useAuthContext,
} from "contexts/auth/AuthProvider";

type RequireKeys<T, R extends keyof T> = Omit<T, R> & {
	[K in keyof Pick<T, R>]-?: NonNullable<T[K]>;
};

// We can do some TS magic here but I would rather to be explicit on what
// values are not undefined when authenticated
type AuthenticatedAuthContextValue = RequireKeys<
	AuthContextValue,
	"user" | "permissions"
>;

export const useAuthenticated = (): AuthenticatedAuthContextValue => {
	const auth = useAuthContext();

	if (!auth.user) {
		throw new Error("User is not authenticated.");
	}

	if (!auth.permissions) {
		throw new Error("Permissions are not available.");
	}

	return auth as AuthenticatedAuthContextValue;
};
