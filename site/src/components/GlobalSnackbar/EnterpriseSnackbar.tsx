import IconButton from "@mui/material/IconButton";
import Snackbar, {
	type SnackbarProps as MuiSnackbarProps,
} from "@mui/material/Snackbar";
import { X as XIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

type EnterpriseSnackbarVariant = "error" | "info" | "success";

interface EnterpriseSnackbarProps extends MuiSnackbarProps {
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
	return (
		<Snackbar
			anchorOrigin={{
				vertical: "bottom",
				horizontal: "right",
			}}
			action={
				<div className="flex items-center">
					{action}
					<IconButton onClick={onClose} className="p-0">
						<XIcon
							aria-label="close"
							className="size-icon-sm text-content-primary"
						/>
					</IconButton>
				</div>
			}
			ContentProps={{
				...ContentProps,
				className: cn(
					"rounded-lg bg-surface-secondary text-content-primary shadow",
					"py-2 pl-6 pr-4 items-[inherit] border-0 border-l-[4px]",
					variantColor(variant),
				),
			}}
			onClose={onClose}
			{...snackbarProps}
		>
			{children}
		</Snackbar>
	);
};

const variantColor = (variant: EnterpriseSnackbarVariant) => {
	switch (variant) {
		case "error":
			return "border-border-destructive";
		case "info":
			return "border-highlight-sky";
		case "success":
			return "border-border-success";
	}
};
