import { useQuery } from "react-query";
import { checkAuthorization } from "#/api/queries/authCheck";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";

/** Chat management permission keyed by organization ID. */
export type UpdateAnyChatByOrganization = Readonly<Record<string, boolean>>;

/**
 * Whether the current user may manage this chat: pin, rename, archive,
 * change its workspace, or send messages. Every one of those writes goes
 * through the server's `chat:update` check, so the UI mirrors it. Owners
 * always pass that check for their own chats; the organization map covers
 * roles such as owner and organization admin whose `chat:update` is not
 * scoped to their own chats. Viewers of a shared chat hold only read access
 * and fail both.
 */
export const canManageChat = (
	chat: TypesGen.Chat,
	currentUserId: string,
	updateAnyChatByOrganization: UpdateAnyChatByOrganization | undefined,
): boolean =>
	chat.owner_id === currentUserId ||
	Boolean(updateAnyChatByOrganization?.[chat.organization_id]);

export const chatUpdateChecks = (
	organizationIds: readonly string[],
): TypesGen.AuthorizationRequest["checks"] =>
	Object.fromEntries(
		organizationIds.map((organizationId) => [
			organizationId,
			{
				// No owner_id: asks whether the user may update every chat
				// in the organization, not just their own.
				object: { resource_type: "chat", organization_id: organizationId },
				action: "update",
			},
		]),
	);

/**
 * Resolves {@link canManageChat} for the current user across the
 * organizations they belong to with a single authorization request.
 * Until that request resolves, only chat owners are treated as managers.
 */
export const useCanManageChat = (): ((chat: TypesGen.Chat) => boolean) => {
	const { user } = useAuthenticated();
	const { organizations } = useDashboard();
	const organizationIds = organizations.map((organization) => organization.id);
	const permissionsQuery = useQuery({
		...checkAuthorization<UpdateAnyChatByOrganization>({
			checks: chatUpdateChecks(organizationIds),
		}),
		enabled: organizationIds.length > 0,
	});
	const updateAnyChatByOrganization = permissionsQuery.data;
	return (chat) => canManageChat(chat, user.id, updateAnyChatByOrganization);
};
