import { type FC, useId } from "react";
import { visuallyHidden } from "@mui/utils";
import type { WorkspaceResource } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { getResourceIconPath } from "utils/workspace";

export type ResourceAvatarProps = { resource: WorkspaceResource };

export const ResourceAvatar: FC<ResourceAvatarProps> = ({ resource }) => {
  const avatarSrc = resource.icon || getResourceIconPath(resource.type);
  const altId = useId();

  return (
    <Avatar background>
      <ExternalImage
        src={avatarSrc}
        css={{ maxWidth: "50%" }}
        aria-labelledby={altId}
      />
      <div id={altId} css={{ ...visuallyHidden }}>
        {resource.name}
      </div>
    </Avatar>
  );
};
