import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	type ButtonHTMLAttributes,
	type ReactNode,
	useEffect,
	useState,
} from "react";

type PaginationNavButtonProps = Omit<
	ButtonHTMLAttributes<HTMLButtonElement>,
	| "aria-disabled"
	// Need to omit color for MUI compatibility
	| "color"
> & {
	// Required/narrowed versions of default props
	children: ReactNode;
	disabled: boolean;
	onClick: () => void;
	"aria-label": string;

	// Bespoke props
	disabledMessage: ReactNode;
	disabledMessageTimeout?: number;
};

function PaginationNavButtonCore({
	onClick,
	disabled,
	disabledMessage,
	disabledMessageTimeout = 3000,
	...delegatedProps
}: PaginationNavButtonProps) {
	const [showDisabledMessage, setShowDisabledMessage] = useState(false);

	// Inline state sync - this is safe/recommended by the React team in this case
	if (!disabled && showDisabledMessage) {
		setShowDisabledMessage(false);
	}

	useEffect(() => {
		if (!showDisabledMessage) {
			return;
		}

		const timeoutId = setTimeout(
			() => setShowDisabledMessage(false),
			disabledMessageTimeout,
		);

		return () => clearTimeout(timeoutId);
	}, [showDisabledMessage, disabledMessageTimeout]);

	return (
		<TooltipProvider delayDuration={100}>
			<Tooltip>
				<TooltipTrigger asChild>
					{/*
					 * Going more out of the way to avoid attaching the disabled prop directly
					 * to avoid unwanted side effects of using the prop:
					 * - Not being focusable/keyboard-navigable
					 * - Not being able to call functions in response to invalid actions
					 *   (mostly for giving direct UI feedback to those actions)
					 */}
					<Button
						variant="outline"
						size="icon"
						disabled={disabled}
						onClick={onClick}
						{...delegatedProps}
					/>
				</TooltipTrigger>
				<TooltipContent>{disabledMessage}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}

export function PaginationNavButton({
	disabledMessageTimeout = 3000,
	...delegatedProps
}: PaginationNavButtonProps) {
	return (
		// Key prop ensures that if timeout changes, the component just unmounts and
		// remounts, avoiding a swath of possible sync issues
		<PaginationNavButtonCore
			key={disabledMessageTimeout}
			disabledMessageTimeout={disabledMessageTimeout}
			{...delegatedProps}
		/>
	);
}
