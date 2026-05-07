import { useTheme } from "@emotion/react";
import {
	cloneElement,
	type FC,
	type HTMLAttributes,
	type ReactElement,
	type Ref,
} from "react";
import { Avatar, type AvatarProps } from "#/components/Avatar/Avatar";
import { Button, type ButtonProps } from "#/components/Button/Button";
import { cn } from "#/utils/cn";

export const Topbar: FC<HTMLAttributes<HTMLElement>> = ({
	className,
	...props
}) => {
	return (
		<header
			{...props}
			className={cn(
				"min-h-12 border-0 border-b border-border border-solid flex items-center text-[13px] leading-tight",
				className,
			)}
		/>
	);
};

type TopbarIconButtonProps = ButtonProps;

export const TopbarIconButton = ({
	className,
	...props
}: TopbarIconButtonProps) => {
	return (
		<Button
			{...props}
			size="icon-lg"
			variant="subtle"
			className={cn("p-0 rounded-none size-12", className)}
		/>
	);
};

export const TopbarButton: React.FC<ButtonProps> = ({ ...props }) => {
	return <Button variant="outline" size="sm" {...props} />;
};

export const TopbarData: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return <div {...props} className="flex gap-2 items-center justify-center" />;
};

export const TopbarDivider: FC<
	Omit<HTMLAttributes<HTMLSpanElement>, "children">
> = (props) => {
	const theme = useTheme();
	return (
		<span {...props} css={{ color: theme.palette.divider }}>
			/
		</span>
	);
};

export const TopbarAvatar: FC<AvatarProps> = (props) => {
	return <Avatar {...props} variant="icon" size="sm" />;
};

type TopbarIconProps = HTMLAttributes<HTMLOrSVGElement> & {
	ref?: Ref<HTMLOrSVGElement>;
};

export const TopbarIcon: React.FC<TopbarIconProps> = ({
	ref,
	children,
	className,
	...restProps
}) => {
	return cloneElement(children as ReactElement<TopbarIconProps>, {
		...restProps,
		ref,
		className: "text-base text-content-disabled size-icon-sm",
	});
};
