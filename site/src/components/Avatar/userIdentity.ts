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

// Builds the title, subtitle, and avatar src for a user. Service accounts get
// the fixed "Service Account" subtitle; otherwise the email is used, falling
// back to undefined when empty.
export function userIdentity(user: UserIdentityInput): UserIdentity {
	return {
		title: user.username,
		subtitle: user.is_service_account
			? "Service Account"
			: user.email || undefined,
		src: user.avatar_url || undefined,
	};
}
