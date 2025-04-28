import type { Interpolation, Theme } from "@emotion/react";
import { visuallyHidden } from "@mui/utils";
import { type FC, type KeyboardEvent, type MouseEvent, useRef } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { CopyButton } from "../CopyButton/CopyButton";

export interface CodeExampleProps {
	code: string;
	secret?: boolean;
	className?: string;
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<CodeExampleProps> = ({
	code,
	className,

	// Defaulting to true to be on the safe side; you should have to opt out of
	// the secure option, not remember to opt in
	secret = true,
}) => {
	const buttonRef = useRef<HTMLButtonElement>(null);
	const triggerButton = (event: KeyboardEvent | MouseEvent) => {
		const clickTriggeredOutsideButton =
			event.target instanceof HTMLElement &&
			!buttonRef.current?.contains(event.target);

		if (clickTriggeredOutsideButton) {
			buttonRef.current?.click();
		}
	};

	return (
		<div
			css={styles.container}
			className={className}
			onClick={triggerButton}
			onKeyDown={(event) => {
				if (event.key === "Enter") {
					triggerButton(event);
				}
			}}
			onKeyUp={(event) => {
				if (event.key === " ") {
					triggerButton(event);
				}
			}}
		>
			<code css={[styles.code, secret && styles.secret]}>
				{secret ? (
					<>
						{/*
						 * Obfuscating text even though we have the characters replaced with
						 * discs in the CSS for two reasons:
						 * 1. The CSS property is non-standard and won't work everywhere;
						 *    MDN warns you not to rely on it alone in production
						 * 2. Even with it turned on and supported, the plaintext is still
						 *    readily available in the HTML itself
						 */}
						<span aria-hidden>{obfuscateText(code)}</span>
						<span css={{ ...visuallyHidden }}>
							Encrypted text. Please access via the copy button.
						</span>
					</>
				) : (
					code
				)}
			</code>

			<CopyButton ref={buttonRef} text={code} />
		</div>
	);
};

function obfuscateText(text: string): string {
	return new Array(text.length).fill("*").join("");
}

const styles = {
	container: (theme) => ({
		cursor: "pointer",
		display: "flex",
		flexDirection: "row",
		alignItems: "center",
		color: theme.experimental.l1.text,
		fontFamily: MONOSPACE_FONT_FAMILY,
		fontSize: 14,
		borderRadius: 8,
		padding: 8,
		lineHeight: "150%",
		border: `1px solid ${theme.experimental.l1.outline}`,

		"&:hover": {
			backgroundColor: theme.experimental.l2.hover.background,
		},
	}),

	code: {
		padding: "0 8px",
		flexGrow: 1,
		wordBreak: "break-all",
	},

	secret: {
		"-webkit-text-security": "disc", // also supported by firefox
	},
} satisfies Record<string, Interpolation<Theme>>;
