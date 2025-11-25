import { css, type Interpolation, type Theme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { EyeIcon, EyeOffIcon } from "lucide-react";
import { type FC, useState } from "react";

const Language = {
	showLabel: "Show value",
	hideLabel: "Hide value",
};

interface SensitiveValueProps {
	value: string;
}

export const SensitiveValue: FC<SensitiveValueProps> = ({ value }) => {
	const [shouldDisplay, setShouldDisplay] = useState(false);
	const displayValue = shouldDisplay ? value : "••••••••";
	const buttonLabel = shouldDisplay ? Language.hideLabel : Language.showLabel;
	const icon = shouldDisplay ? (
		<EyeOffIcon className="size-icon-xs" />
	) : (
		<EyeIcon className="size-icon-xs" />
	);

	return (
		<div
			css={{
				display: "flex",
				alignItems: "center",
				gap: 4,
			}}
		>
			<CopyableValue
				value={value}
				className="w-[calc(100%-22px)] overflow-hidden whitespace-nowrap text-ellipsis"
			>
				{displayValue}
			</CopyableValue>
			<Tooltip>
				<TooltipTrigger asChild>
					<IconButton
						css={styles.button}
						onClick={() => {
							setShouldDisplay((value) => !value);
						}}
						size="small"
						aria-label={buttonLabel}
					>
						{icon}
					</IconButton>
				</TooltipTrigger>
				<TooltipContent side="bottom">{buttonLabel}</TooltipContent>
			</Tooltip>
		</div>
	);
};

const styles = {
	button: css`
		color: inherit;
	`,
} satisfies Record<string, Interpolation<Theme>>;
