import { CloseIcon as CloseIcon } from "lucide-react";
import type { Interpolation, Theme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import Snackbar, {
	type SnackbarProps as MuiSnackbarProps,
} from "@mui/material/Snackbar";
import { type ClassName, useClassName } from "hooks/useClassName";
import type { FC } from "react";

type EnterpriseSnackbarVariant = "error" | "info" | "success";

export interface EnterpriseSnackbarProps extends MuiSnackbarProps {
	/** Called when the snackbar should close, either from timeout or clicking close */
	onClose: () => void;
	/** Variant of snackbar, for theming */
	variant?: EnterpriseSnackbarVariant;
}

/**
 * Wrapper around Material UI's Snackbar component, provides pre-configured
 * themes and convenience props. Coder UI's Snackbars require a close handler,
 * since they always render a close button.
 *
 * Snackbars do _not_ automatically appear in the top-level position when
 * rendered, you'll need to use ReactDom portals or the Material UI Portal
 * component for that.
 *
 * See original component's Material UI documentation here: https://material-ui.com/components/snackbars/
 */
export const EnterpriseSnackbar: FC<EnterpriseSnackbarProps> = ({
	children,
	onClose,
	variant = "info",
	ContentProps = {},
	action,
	...snackbarProps
}) => {
	const content = useClassName(classNames.content(variant), [variant]);

	return (
		<Snackbar
			anchorOrigin={{
				vertical: "bottom",
				horizontal: "right",
			}}
			action={
				<div css={styles.actionWrapper}>
					{action}
					<IconButton onClick={onClose} css={{ padding: 0 }}>
						<CloseIcon css={styles.closeIcon} aria-label="close" />
					</IconButton>
				</div>
			}
			ContentProps={{
				...ContentProps,
				className: content,
			}}
			onClose={onClose}
			{...snackbarProps}
		>
			{children}
		</Snackbar>
	);
};

const variantColor = (variant: EnterpriseSnackbarVariant, theme: Theme) => {
	switch (variant) {
		case "error":
			return theme.palette.error.main;
		case "info":
			return theme.palette.info.main;
		case "success":
			return theme.palette.success.main;
	}
};

const classNames = {
	content:
		(variant: EnterpriseSnackbarVariant): ClassName =>
		(css, theme) =>
			css`
      border: 1px solid ${theme.palette.divider};
      border-left: 4px solid ${variantColor(variant, theme)};
      border-radius: 8px;
      padding: 8px 24px 8px 16px;
      box-shadow: ${theme.shadows[6]};
      align-items: inherit;
      background-color: ${theme.palette.background.paper};
      color: ${theme.palette.text.secondary};
    `,
};

const styles = {
	actionWrapper: {
		display: "flex",
		alignItems: "center",
	},
	closeIcon: (theme) => ({
		width: 25,
		height: 25,
		color: theme.palette.primary.contrastText,
	}),
} satisfies Record<string, Interpolation<Theme>>;
