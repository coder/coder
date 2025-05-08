import { CheckCircleOutlined as CheckCircleOutlined, CloseIcon as CloseIcon, DoNotDisturbOnOutlined as DoNotDisturbOnOutlined, Sell as Sell } from "lucide-react";
import type { Interpolation, Theme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import { Pill } from "components/Pill/Pill";
import type { ComponentProps, FC } from "react";

const parseBool = (s: string): { valid: boolean; value: boolean } => {
	switch (s.toLowerCase()) {
		case "true":
		case "yes":
		case "1":
			return { valid: true, value: true };
		case "false":
		case "no":
		case "0":
		case "":
			return { valid: true, value: false };
		default:
			return { valid: false, value: false };
	}
};

interface ProvisionerTagProps {
	tagName: string;
	tagValue: string;
	/** Only used in the TemplateVersionEditor */
	onDelete?: (tagName: string) => void;
}

export const ProvisionerTag: FC<ProvisionerTagProps> = ({
	tagName,
	tagValue,
	onDelete,
}) => {
	const { valid, value: boolValue } = parseBool(tagValue);
	const kv = (
		<>
			<span css={{ fontWeight: 600 }}>{tagName}</span> <span>{tagValue}</span>
		</>
	);
	const content = onDelete ? (
		<>
			{kv}
			<IconButton
				size="small"
				color="secondary"
				onClick={() => {
					onDelete(tagName);
				}}
			>
				<CloseIcon fontSize="inherit" css={{ width: 14, height: 14 }} />
				<span className="sr-only">Delete {tagName}</span>
			</IconButton>
		</>
	) : (
		kv
	);
	if (valid) {
		return <BooleanPill value={boolValue}>{content}</BooleanPill>;
	}
	return (
		<Pill size="lg" icon={<Sell />} data-testid={`tag-${tagName}`}>
			{content}
		</Pill>
	);
};

type BooleanPillProps = Omit<ComponentProps<typeof Pill>, "icon" | "value"> & {
	value: boolean;
};

const BooleanPill: FC<BooleanPillProps> = ({
	value,
	children,
	...divProps
}) => {
	return (
		<Pill
			type={value ? "active" : "danger"}
			size="lg"
			icon={
				value ? (
					<CheckCircleOutlined css={styles.truePill} />
				) : (
					<DoNotDisturbOnOutlined css={styles.falsePill} />
				)
			}
			{...divProps}
		>
			{children}
		</Pill>
	);
};

const styles = {
	truePill: (theme) => ({
		color: theme.roles.active.outline,
	}),
	falsePill: (theme) => ({
		color: theme.roles.danger.outline,
	}),
} satisfies Record<string, Interpolation<Theme>>;
