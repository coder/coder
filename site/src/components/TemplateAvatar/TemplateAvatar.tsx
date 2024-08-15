import type { Template } from "api/typesGenerated";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import type { FC } from "react";

interface TemplateAvatarProps extends AvatarProps {
	template: Template;
}

export const TemplateAvatar: FC<TemplateAvatarProps> = ({
	template,
	...avatarProps
}) => {
	return template.icon ? (
		<Avatar src={template.icon} variant="square" fitImage {...avatarProps} />
	) : (
		<Avatar {...avatarProps}>{template.display_name || template.name}</Avatar>
	);
};
