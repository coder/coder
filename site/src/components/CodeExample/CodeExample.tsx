import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { EyeIcon, EyeOffIcon } from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { CopyButton } from "../CopyButton/CopyButton";

interface CodeExampleProps {
	code: string;
	/** Defaulting to true to be on the safe side; you should have to opt out of the secure option, not remember to opt in */
	secret?: boolean;
	/** Redact parts of the code if the user doesn't want to obfuscate the whole code */
	redactPattern?: RegExp;
	/** Replacement text for redacted content */
	redactReplacement?: string;
	/** Show a button to reveal the redacted parts of the code */
	showRevealButton?: boolean;
	className?: string;
}

/**
 * Component to show single-line code examples, with a copy button
 */
export const CodeExample: FC<CodeExampleProps> = ({
	code,
	className,
	secret = true,
	redactPattern,
	redactReplacement = "********",
	showRevealButton,
}) => {
	const [showFullValue, setShowFullValue] = useState(false);

	const displayValue = secret
		? obfuscateText(code)
		: redactPattern && !showFullValue
			? code.replace(redactPattern, redactReplacement)
			: code;

	const showButtonLabel = showFullValue
		? "Hide sensitive data"
		: "Show sensitive data";
	const icon = showFullValue ? (
		<EyeOffIcon className="h-4 w-4" />
	) : (
		<EyeIcon className="h-4 w-4" />
	);

	return (
		<div
			className={cn(
				"cursor-pointer flex flex-row items-center text-sm font-mono rounded-lg p-2",
				"leading-normal text-content-primary border border-solid",
				"border-gray-300 hover:bg-gray-200",
				"dark:bg-zinc-800 dark:border-zinc-700 dark:hover:bg-zinc-800",
				className,
			)}
		>
			<code
				className={cn(
					"px-2 flex-grow break-all",
					secret && "[text-security:disc] [-webkit-text-security:disc]",
				)}
			>
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
						<span aria-hidden>{displayValue}</span>
						<span className="sr-only">
							Encrypted text. Please access via the copy button.
						</span>
					</>
				) : (
					displayValue
				)}
			</code>

			<div className="flex items-center gap-1">
				{showRevealButton && redactPattern && !secret && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								onClick={() => setShowFullValue(!showFullValue)}
							>
								{icon}
								<span className="sr-only">{showButtonLabel}</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>{showButtonLabel}</TooltipContent>
					</Tooltip>
				)}
				<CopyButton text={code} label="Copy code" />
			</div>
		</div>
	);
};

function obfuscateText(text: string): string {
	return new Array(text.length).fill("*").join("");
}
