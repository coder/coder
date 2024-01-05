import { type FC } from "react";
import type { WorkspaceResource } from "api/typesGenerated";
import { Avatar, AvatarIcon } from "components/Avatar/Avatar";
import { getResourceIconPath } from "utils/workspace";

export type ResourceAvatarProps = { resource: WorkspaceResource };

export const ResourceAvatar: FC<ResourceAvatarProps> = ({ resource }) => {
  const avatarSrc = resource.icon || getResourceIconPath(resource.type);

  return (
    <Avatar background>
      <AvatarIcon src={avatarSrc} alt={resource.name} />
    </Avatar>
  );
};
