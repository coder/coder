import type { FC } from "react";
import type { Template } from "api/typesGenerated";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";

type TemplateAvatarProps = {
  template: Template;
  size: AvatarProps["size"];
};

export const TemplateAvatar: FC<TemplateAvatarProps> = ({ template, size }) => {
  const hasIcon = template.icon && template.icon !== "";

  if (hasIcon) {
    return <Avatar size={size} src={template.icon} variant="square" fitImage />;
  }
  return <Avatar size={size}>{template.name}</Avatar>;
};
