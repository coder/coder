import type { ChatUsageLimitGroupOverride } from "#/api/typesGenerated";

export const GROUP_LIMITS_PAGE_SIZE = 10;

export interface GroupLimitOverrideGroup {
	group_id: string;
	group_display_name: string;
	group_name: string;
	group_avatar_url: string;
	member_count: number;
}

export type GroupLimitOverride = ChatUsageLimitGroupOverride;
