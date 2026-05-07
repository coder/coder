import type { FC } from "react";
import type { WorkspaceResource } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { getResourceIconPath } from "#/utils/workspace";

type ResourceAvatarProps = { resource: WorkspaceResource };

export const ResourceAvatar: FC<ResourceAvatarProps> = ({ resource }) => {
	const avatarSrc = resource.icon || getResourceIconPath(resource.type);

	return <Avatar variant="icon" src={avatarSrc} fallback={resource.name} />;
};
