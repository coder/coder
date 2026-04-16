import type { AuthorizationCheck, Chat } from "#/api/typesGenerated";

// chatChecks builds the authorization checks for a single chat. The
// shape mirrors workspaceChecks so callers can drive useAuthorization
// with the same pattern. ``shareChat`` is gated by the `chat:share`
// RBAC action that the backend introduced alongside the ACL feature.
export const chatChecks = (chat: Chat) =>
	({
		readChat: {
			object: {
				resource_type: "chat",
				resource_id: chat.id,
				owner_id: chat.owner_id,
			},
			action: "read",
		},
		shareChat: {
			object: {
				resource_type: "chat",
				resource_id: chat.id,
				owner_id: chat.owner_id,
			},
			action: "share",
		},
		updateChat: {
			object: {
				resource_type: "chat",
				resource_id: chat.id,
				owner_id: chat.owner_id,
			},
			action: "update",
		},
	}) satisfies Record<string, AuthorizationCheck>;

export type ChatPermissions = Record<
	keyof ReturnType<typeof chatChecks>,
	boolean
>;
