import { type Interpolation, type Theme, css } from "@emotion/react";
import IconButton from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import { useClipboard } from "hooks/useClipboard";
import { CheckIcon } from "lucide-react";
import { type ReactNode, forwardRef } from "react";
import { FileCopyIcon } from "../Icons/FileCopyIcon";

interface CopyButtonProps {
	children?: ReactNode;
	text: string;
	ctaCopy?: string;
	wrapperStyles?: Interpolation<Theme>;
	buttonStyles?: Interpolation<Theme>;
	tooltipTitle?: string;
}

const Language = {
	tooltipTitle: "Copy to clipboard",
	ariaLabel: "Copy to clipboard",
};

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton = forwardRef<HTMLButtonElement, CopyButtonProps>(
	(props, ref) => {
		const {
			text,
			ctaCopy,
			wrapperStyles,
			buttonStyles,
			tooltipTitle = Language.tooltipTitle,
		} = props;
		const { showCopiedSuccess, copyToClipboard } = useClipboard({
			textToCopy: text,
		});

		return (
			<Tooltip title={tooltipTitle} placement="top">
				<div css={[{ display: "flex" }, wrapperStyles]}>
					<IconButton
						ref={ref}
						css={[styles.button, buttonStyles]}
						size="small"
						aria-label={Language.ariaLabel}
						variant="text"
						onClick={copyToClipboard}
					>
						{showCopiedSuccess ? (
							<CheckIcon css={styles.copyIcon} />
						) : (
							<FileCopyIcon css={styles.copyIcon} />
						)}
						{ctaCopy && <div css={{ marginLeft: 8 }}>{ctaCopy}</div>}
					</IconButton>
				</div>
			</Tooltip>
		);
	},
);

const styles = {
	button: (theme) => css`
    border-radius: 8px;
    padding: 8px;
    min-width: 32px;

    &:hover {
      background: ${theme.palette.background.paper};
    }
  `,
	copyIcon: css`
    width: 20px;
    height: 20px;
  `,
} satisfies Record<string, Interpolation<Theme>>;
