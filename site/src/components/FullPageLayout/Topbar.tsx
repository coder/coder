import { useTheme } from "@emotion/react";
import IconButton, { type IconButtonProps } from "@mui/material/IconButton";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import { Button, type ButtonProps } from "components/Button/Button";
import {
	cloneElement,
	type FC,
	type ForwardedRef,
	forwardRef,
	type HTMLAttributes,
	type ReactElement,
} from "react";

export const Topbar: FC<HTMLAttributes<HTMLElement>> = (props) => {
	return (
		<header
			className="min-h-12 border-0 border-b border-border border-solid flex items-center text-[13px] leading-tight"
			{...props}
		/>
	);
};

export const TopbarIconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
	(props, ref) => {
		return (
			<IconButton
				ref={ref}
				{...props}
				size="small"
				className="p-0 rounded-none size-12 [&_svg]:size-icon-sm"
			/>
		);
	},
) as typeof IconButton;

export const TopbarButton = forwardRef<HTMLButtonElement, ButtonProps>(
	(props: ButtonProps, ref) => {
		return <Button ref={ref} variant="outline" size="sm" {...props} />;
	},
);

export const TopbarData: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	return <div {...props} className="flex gap-2 items-center justify-center" />;
};

export const TopbarDivider: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	const theme = useTheme();
	return (
		<span {...props} css={{ color: theme.palette.divider }}>
			/
		</span>
	);
};

export const TopbarAvatar: FC<AvatarProps> = (props) => {
	return <Avatar {...props} variant="icon" size="md" />;
};

type TopbarIconProps = HTMLAttributes<HTMLOrSVGElement>;

export const TopbarIcon = forwardRef<HTMLOrSVGElement, TopbarIconProps>(
	(props: TopbarIconProps, ref) => {
		const { children, className, ...restProps } = props;

		return cloneElement(
			children as ReactElement<
				HTMLAttributes<HTMLOrSVGElement> & {
					ref: ForwardedRef<HTMLOrSVGElement>;
				}
			>,
			{
				...restProps,
				ref,
				className: "text-base text-content-disabled size-icon-sm",
			},
		);
	},
);
