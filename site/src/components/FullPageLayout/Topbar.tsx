import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import IconButton, { type IconButtonProps } from "@mui/material/IconButton";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import { Button, type ButtonProps } from "components/Button/Button";
import {
	type FC,
	type ForwardedRef,
	type HTMLAttributes,
	type ReactElement,
	cloneElement,
	forwardRef,
} from "react";

export const Topbar: FC<HTMLAttributes<HTMLElement>> = (props) => {
	const theme = useTheme();

	return (
		<header
			{...props}
			css={{
				minHeight: 48,
				borderBottom: `1px solid ${theme.palette.divider}`,
				display: "flex",
				alignItems: "center",
				fontSize: 13,
				lineHeight: "1.2",
			}}
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
					padding: 0,
					borderRadius: 0,
					height: 48,
					width: 48,

					"& svg": {
						fontSize: 20,
					},
				}}
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
	return (
		<div
			{...props}
			css={{
				display: "flex",
				gap: 8,
				alignItems: "center",
				justifyContent: "center",
			}}
		/>
	);
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
		const { children, ...restProps } = props;
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
				className: css({ fontSize: 16, color: theme.palette.text.disabled }),
			},
		);
	},
);
