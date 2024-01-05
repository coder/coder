import { type FC, useId } from "react";
import { visuallyHidden } from "@mui/utils";
import type { WorkspaceResource } from "api/typesGenerated";
import { Avatar, AvatarIcon } from "components/Avatar/Avatar";
import { ExternalImage } from "components/ExternalImage/ExternalImage";

const FALLBACK_ICON = "/icon/widgets.svg";

// These resources (i.e. docker_image, kubernetes_deployment) map to Terraform
// resource types. These are the most used ones and are based on user usage.
// We may want to update from time-to-time.
const BUILT_IN_ICON_PATHS: Record<string, string> = {
  docker_volume: "/icon/database.svg",
  docker_container: "/icon/memory.svg",
  docker_image: "/icon/container.svg",
  kubernetes_persistent_volume_claim: "/icon/database.svg",
  kubernetes_pod: "/icon/memory.svg",
  google_compute_disk: "/icon/database.svg",
  google_compute_instance: "/icon/memory.svg",
  aws_instance: "/icon/memory.svg",
  kubernetes_deployment: "/icon/memory.svg",
};

export const getIconPathResource = (resourceType: string): string => {
  return BUILT_IN_ICON_PATHS[resourceType] ?? FALLBACK_ICON;
};

export type ResourceAvatarProps = { resource: WorkspaceResource };

export const ResourceAvatar: FC<ResourceAvatarProps> = ({ resource }) => {
  const avatarSrc = resource.icon || getIconPathResource(resource.type);
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
