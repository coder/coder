import { css } from "@emotion/css";
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
import { cn } from "utils/cn";

export const Topbar: FC<HTMLAttributes<HTMLElement>> = (props) => {
	const theme = useTheme();

	return (
		<header
			{...props}
			css={{
				borderBottom: `1px solid ${theme.palette.divider}`,
				fontSize: 13,
				lineHeight: "1.2",
			}}
			className={cn("min-h-12 flex items-center", props.className)}
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
				css={{
					"& svg": {
						fontSize: 20,
					},
				}}
				className={cn("p-0 rounded-none size-12", props.className)}
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
	return <Avatar {...props} variant="icon" size="sm" />;
};

type TopbarIconProps = HTMLAttributes<HTMLOrSVGElement>;

export const TopbarIcon = forwardRef<HTMLOrSVGElement, TopbarIconProps>(
	(props: TopbarIconProps, ref) => {
		const { children, className, ...restProps } = props;
		const theme = useTheme();

		return cloneElement(
			children as ReactElement<
				HTMLAttributes<HTMLOrSVGElement> & {
					ref: ForwardedRef<HTMLOrSVGElement>;
				}
			>,
			{
				...restProps,
				ref,
				className: cn([
					css({ fontSize: 16, color: theme.palette.text.disabled }),
					"size-icon-sm",
				]),
			},
		);
	},
);
