export const USER_OVERRIDES_PAGE_SIZE = 10;

export interface UserOverrideUser {
	user_id: string;
	name: string;
	username: string;
	avatar_url: string;
}

export interface UserOverride extends UserOverrideUser {
	spend_limit_micros: number | null;
}
