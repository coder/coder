import { VisibilityOffOutlined as VisibilityOffOutlined, VisibilityOutlined as VisibilityOutlined } from "lucide-react";
import { type Interpolation, type Theme, css } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
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
		<VisibilityOffOutlined />
	) : (
		<VisibilityOutlined />
	);

	return (
		<div
			css={{
				display: "flex",
				alignItems: "center",
				gap: 4,
			}}
		>
			<CopyableValue value={value} css={styles.value}>
				{displayValue}
			</CopyableValue>
			<Tooltip title={buttonLabel}>
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
			</Tooltip>
		</div>
	);
};

const styles = {
	value: {
		// 22px is the button width
		width: "calc(100% - 22px)",
		overflow: "hidden",
		whiteSpace: "nowrap",
		textOverflow: "ellipsis",
	},

	button: css`
    color: inherit;

    & .MuiSvgIcon-root {
      width: 16px;
      height: 16px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;
