type UserIdentityInput = {
	username: string;
	email?: string;
	avatar_url?: string;
	is_service_account?: boolean;
};

type UserIdentity = {
	title: string;
	subtitle: string | undefined;
	src: string | undefined;
};

// Username is primary; email disambiguates. Service accounts have no login
// email, so they get a fixed subtitle instead.
export function userIdentity(user: UserIdentityInput): UserIdentity {
	return {
		title: user.username,
		subtitle: user.is_service_account
			? "Service Account"
			: user.email || undefined,
		src: user.avatar_url || undefined,
	};
}
