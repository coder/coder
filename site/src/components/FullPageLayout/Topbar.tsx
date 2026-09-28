import { cn } from "cn";
import { cloneElement } from "react";
import { Avatar, type AvatarProps } from "#/components/Avatar/Avatar";
import { Button, type ButtonProps } from "#/components/Button/Button";

export const Topbar: React.FC<React.ComponentProps<"header">> = ({
	className,
	...props
}) => {
	return (
		<header
			{...props}
			className={cn(
				"min-h-12 border-0 border-b border-border border-solid flex items-center text-sm font-normal leading-tight",
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

export const TopbarData: React.FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			{...props}
			className={cn("flex gap-2 items-center justify-center", className)}
		/>
	);
};

export const TopbarDivider: React.FC<
	Omit<React.ComponentProps<"span">, "children">
> = ({ className, ...props }) => {
	return (
		<span {...props} className={cn("text-border", className)}>
			/
		</span>
	);
};

export const TopbarAvatar: React.FC<AvatarProps> = (props) => {
	return <Avatar {...props} variant="icon" size="sm" />;
};

// oxlint-disable-next-line no-restricted-types
type TopbarIconProps = React.HTMLAttributes<HTMLOrSVGElement> & {
	ref?: React.Ref<HTMLOrSVGElement>;
};

export const TopbarIcon: React.FC<TopbarIconProps> = ({
	ref,
	children,
	className,
	...restProps
}) => {
	return cloneElement(children as React.ReactElement<TopbarIconProps>, {
		...restProps,
		ref,
		className: "text-base text-content-disabled size-icon-sm",
	});
};
