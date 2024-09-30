import { type Interpolation, type Theme, css, useTheme } from "@emotion/react";
import BuildCircleOutlinedIcon from "@mui/icons-material/BuildCircleOutlined";
import { DisabledBadge, EnabledBadge } from "components/Badges/Badges";
import type { FC, HTMLAttributes, PropsWithChildren } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

export const OptionName: FC<PropsWithChildren> = ({ children }) => {
	return <span css={{ display: "block" }}>{children}</span>;
};

export const OptionDescription: FC<PropsWithChildren> = ({ children }) => {
	const theme = useTheme();

	return (
		<span
			css={{
				display: "block",
				color: theme.palette.text.secondary,
				fontSize: 14,
				marginTop: 4,
			}}
		>
			{children}
		</span>
	);
};

interface OptionValueProps {
	children?: boolean | number | string | string[] | Record<string, boolean>;
}

export const OptionValue: FC<OptionValueProps> = (props) => {
	const { children: value } = props;
	const theme = useTheme();

	if (typeof value === "boolean") {
		return (
			<div className="option-value-boolean">
				{value ? <EnabledBadge /> : <DisabledBadge />}
			</div>
		);
	}

	if (typeof value === "number") {
		return (
			<span css={styles.option} className="option-value-number">
				{value}
			</span>
		);
	}

	if (!value || value.length === 0) {
		return (
			<span css={styles.option} className="option-value-empty">
				Not set
			</span>
		);
	}

	if (typeof value === "string") {
		return (
			<span css={styles.option} className="option-value-string">
				{value}
			</span>
		);
	}

	if (typeof value === "object" && !Array.isArray(value)) {
		return (
			<ul css={{ listStyle: "none" }} className="option-array">
				{Object.entries(value)
					.sort((a, b) => a[0].localeCompare(b[0]))
					.map(([option, isEnabled]) => (
						<li
							key={option}
							css={[
								styles.option,
								!isEnabled && {
									marginLeft: 32,
									color: theme.palette.text.disabled,
								},
							]}
							className={`option-array-item-${option} ${
								isEnabled ? "option-enabled" : "option-disabled"
							}`}
						>
							<div
								css={{
									display: "inline-flex",
									alignItems: "center",
								}}
							>
								{isEnabled && (
									<BuildCircleOutlinedIcon
										css={(theme) => ({
											width: 16,
											height: 16,
											color: theme.palette.mode,
											margin: "0 8px",
										})}
									/>
								)}
								{option}
							</div>
						</li>
					))}
			</ul>
		);
	}

	if (Array.isArray(value)) {
		return (
			<ul css={{ listStylePosition: "inside" }} className="option-array">
				{value.map((item) => (
					<li key={item} css={styles.option}>
						{item}
					</li>
				))}
			</ul>
		);
	}

	return (
		<span css={styles.option} className="option-value-json">
			{JSON.stringify(value)}
		</span>
	);
};

type OptionConfigProps = HTMLAttributes<HTMLDivElement> & { isSource: boolean };

// OptionConfig takes a isSource bool to indicate if the Option is the source of the configured value.
export const OptionConfig: FC<OptionConfigProps> = ({ isSource, ...attrs }) => {
	return (
		<div
			{...attrs}
			css={[styles.configOption, isSource && styles.sourceConfigOption]}
		/>
	);
};

export const OptionConfigFlag: FC<HTMLAttributes<HTMLDivElement>> = (props) => {
	const theme = useTheme();

	return (
		<div
			{...props}
			className="OptionConfigFlag"
			css={{
				fontSize: 10,
				fontWeight: 600,
				display: "block",
				backgroundColor: theme.palette.divider,
				lineHeight: 1,
				padding: "2px 4px",
				borderRadius: 1,
			}}
		/>
	);
};

const styles = {
	configOption: (theme) => ({
		fontSize: 13,
		fontFamily: MONOSPACE_FONT_FAMILY,
		fontWeight: 600,
		backgroundColor: theme.palette.background.paper,
		display: "inline-flex",
		alignItems: "center",
		borderRadius: 4,
		padding: 6,
		lineHeight: 1,
		gap: 6,
		border: `1px solid ${theme.palette.divider}`,
	}),

	sourceConfigOption: (theme) => ({
		border: `1px solid ${theme.roles.active.fill.outline}`,

		"& .OptionConfigFlag": {
			background: theme.roles.active.fill.solid,
		},
	}),

	option: css`
    font-size: 14px;
    font-family: ${MONOSPACE_FONT_FAMILY};
    overflow-wrap: anywhere;
    user-select: all;

    & ul {
      padding: 16px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;
