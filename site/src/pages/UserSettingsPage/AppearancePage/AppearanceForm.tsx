import type { Interpolation } from "@emotion/react";
import CircularProgress from "@mui/material/CircularProgress";
import FormControl from "@mui/material/FormControl";
import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField from "@mui/material/TextField";
import { visuallyHidden } from "@mui/utils";
import {
	type TerminalFontName,
	TerminalFontNames,
	type UpdateUserAppearanceSettingsRequest,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { PreviewBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import { ThemeOverride } from "contexts/ThemeProvider";
import type { FC } from "react";
import themes, { DEFAULT_THEME, type Theme } from "theme";
import {
	DEFAULT_TERMINAL_FONT,
	DEFAULT_TERMINAL_FONT_SIZE,
	terminalFontLabels,
	terminalFonts,
} from "theme/constants";
import { Section } from "../Section";

export interface AppearanceFormProps {
	isUpdating?: boolean;
	error?: unknown;
	initialValues: UpdateUserAppearanceSettingsRequest;
	onSubmit: (values: UpdateUserAppearanceSettingsRequest) => Promise<unknown>;
}

export const AppearanceForm: FC<AppearanceFormProps> = ({
	isUpdating,
	error,
	onSubmit,
	initialValues,
}) => {
	const currentTheme = initialValues.theme_preference || DEFAULT_THEME;
	const currentTerminalFont =
		initialValues.terminal_font || DEFAULT_TERMINAL_FONT;
	const currentTerminalFontSize =
		initialValues.terminal_font_size || DEFAULT_TERMINAL_FONT_SIZE;

	const onChangeTheme = async (theme: string) => {
		if (isUpdating) {
			return;
		}
		await onSubmit({
			theme_preference: theme,
			terminal_font: currentTerminalFont,
			terminal_font_size: currentTerminalFontSize,
		});
	};

	const onChangeTerminalFont = async (terminalFont: TerminalFontName) => {
		if (isUpdating) {
			return;
		}
		await onSubmit({
			theme_preference: currentTheme,
			terminal_font: terminalFont,
			terminal_font_size: currentTerminalFontSize,
		});
	};

	const onChangeTerminalFontSize = async (terminalFontSize: number) => {
		if (isUpdating) {
			return;
		}
		await onSubmit({
			theme_preference: currentTheme,
			terminal_font: currentTerminalFont,
			terminal_font_size: terminalFontSize,
		});
	};

	return (
		<form>
			{Boolean(error) && (
				<>
					<ErrorAlert error={error} />
					<div css={{ marginBottom: 48 }}></div>
				</>
			)}
			<Section
				title={
					<Stack direction="row" alignItems="center">
						<span>Theme</span>
						{isUpdating && <CircularProgress size={16} />}
					</Stack>
				}
				layout="fluid"
			>
				<Stack direction="row" wrap="wrap">
					<AutoThemePreviewButton
						displayName="Auto"
						active={currentTheme === "auto"}
						themes={[themes.dark, themes.light]}
						onSelect={() => onChangeTheme("auto")}
					/>
					<ThemePreviewButton
						displayName="Dark"
						active={currentTheme === "dark"}
						theme={themes.dark}
						onSelect={() => onChangeTheme("dark")}
					/>
					<ThemePreviewButton
						displayName="Light"
						active={currentTheme === "light"}
						theme={themes.light}
						onSelect={() => onChangeTheme("light")}
					/>
				</Stack>
			</Section>
			<div css={{ marginBottom: 48 }}></div>
			<Section
				title={
					<Stack direction="row" alignItems="center">
						<span>Terminal Font</span>
						{isUpdating && <CircularProgress size={16} />}
					</Stack>
				}
				layout="fluid"
			>
				<FormControl>
					<RadioGroup
						aria-labelledby="fonts-radio-buttons-group-label"
						defaultValue={currentTerminalFont}
						name="fonts-radio-buttons-group"
						onChange={(_, value) =>
							onChangeTerminalFont(toTerminalFontName(value))
						}
					>
						{TerminalFontNames.filter((name) => name !== "").map((name) => (
							<FormControlLabel
								key={name}
								value={name}
								control={<Radio />}
								label={
									<div css={{ fontFamily: terminalFonts[name] }}>
										{terminalFontLabels[name]}
									</div>
								}
							/>
						))}
					</RadioGroup>
				</FormControl>
			</Section>
			<div css={{ marginBottom: 48 }}></div>
			<Section
				title={
					<Stack direction="row" alignItems="center">
						<span>Terminal Font Size</span>
						{isUpdating && <CircularProgress size={16} />}
					</Stack>
				}
				layout="fluid"
			>
				<TextField
					disabled={isUpdating}
					label="Font size (px)"
					variant="outlined"
					type="number"
					size="small"
					style={{ width: "110px" }}
					defaultValue={currentTerminalFontSize}
					inputProps={{ min: 8, max: 32 }}
					onBlur={(e) => {
						onChangeTerminalFontSize(Number(e.target.value));
					}}
					onKeyDown={(e) => {
						if (e.key === "Enter") {
							e.preventDefault(); // Prevent form submission
						}
					}}
				/>
			</Section>
		</form>
	);
};

export function toTerminalFontName(value: string): TerminalFontName {
	return TerminalFontNames.includes(value as TerminalFontName)
		? (value as TerminalFontName)
		: "";
}

interface AutoThemePreviewButtonProps extends Omit<ThemePreviewProps, "theme"> {
	themes: [Theme, Theme];
	onSelect?: () => void;
}

const AutoThemePreviewButton: FC<AutoThemePreviewButtonProps> = ({
	active,
	preview,
	className,
	displayName,
	themes,
	onSelect,
}) => {
	const [leftTheme, rightTheme] = themes;

	return (
		<>
			<input
				type="radio"
				name="theme"
				id={displayName}
				value={displayName}
				checked={active}
				onChange={onSelect}
				css={{ ...visuallyHidden }}
			/>
			<label htmlFor={displayName} className={className}>
				<ThemePreview
					css={{
						// This half is absolute to not advance the layout (which would offset the second half)
						position: "absolute",
						// Slightly past the bounding box to avoid cutting off the outline
						clipPath: "polygon(-5% -5%, 50% -5%, 50% 105%, -5% 105%)",
					}}
					active={active}
					preview={preview}
					displayName={displayName}
					theme={leftTheme}
				/>
				<ThemePreview
					active={active}
					preview={preview}
					displayName={displayName}
					theme={rightTheme}
				/>
			</label>
		</>
	);
};

interface ThemePreviewButtonProps extends ThemePreviewProps {
	onSelect?: () => void;
}

const ThemePreviewButton: FC<ThemePreviewButtonProps> = ({
	active,
	preview,
	className,
	displayName,
	theme,
	onSelect,
}) => {
	return (
		<>
			<input
				type="radio"
				name="theme"
				id={displayName}
				value={displayName}
				checked={active}
				onChange={onSelect}
				css={{ ...visuallyHidden }}
			/>
			<label htmlFor={displayName} className={className}>
				<ThemePreview
					active={active}
					preview={preview}
					displayName={displayName}
					theme={theme}
				/>
			</label>
		</>
	);
};

interface ThemePreviewProps {
	active?: boolean;
	preview?: boolean;
	className?: string;
	displayName: string;
	theme: Theme;
}

const ThemePreview: FC<ThemePreviewProps> = ({
	active,
	preview,
	className,
	displayName,
	theme,
}) => {
	return (
		<ThemeOverride theme={theme}>
			<div
				css={[styles.container, active && styles.containerActive]}
				className={className}
			>
				<div css={styles.page}>
					<div css={styles.header}>
						<div css={styles.headerLinks}>
							<div css={[styles.headerLink, styles.activeHeaderLink]} />
							<div css={styles.headerLink} />
							<div css={styles.headerLink} />
						</div>
						<div css={styles.headerLinks}>
							<div css={styles.proxy} />
							<div css={styles.user} />
						</div>
					</div>
					<div css={styles.body}>
						<div css={styles.title} />
						<div css={styles.table}>
							<div css={styles.tableHeader} />
							<div css={styles.workspace} />
							<div css={styles.workspace} />
							<div css={styles.workspace} />
							<div css={styles.workspace} />
						</div>
					</div>
				</div>
				<div css={styles.label}>
					<span>{displayName}</span>
					{preview && <PreviewBadge />}
				</div>
			</div>
		</ThemeOverride>
	);
};

const styles = {
	container: (theme) => ({
		backgroundColor: theme.palette.background.default,
		border: `1px solid ${theme.palette.divider}`,
		width: 220,
		color: theme.palette.text.primary,
		borderRadius: 6,
		overflow: "clip",
		userSelect: "none",
	}),
	containerActive: (theme) => ({
		outline: `2px solid ${theme.roles.active.outline}`,
	}),
	page: (theme) => ({
		backgroundColor: theme.palette.background.default,
		color: theme.palette.text.primary,
	}),
	header: (theme) => ({
		backgroundColor: theme.palette.background.paper,
		display: "flex",
		alignItems: "center",
		justifyContent: "space-between",
		padding: "6px 10px",
		marginBottom: 8,
		borderBottom: `1px solid ${theme.palette.divider}`,
	}),
	headerLinks: {
		display: "flex",
		alignItems: "center",
		gap: 6,
	},
	headerLink: (theme) => ({
		backgroundColor: theme.palette.text.secondary,
		height: 6,
		width: 20,
		borderRadius: 3,
	}),
	activeHeaderLink: (theme) => ({
		backgroundColor: theme.palette.text.primary,
	}),
	proxy: (theme) => ({
		backgroundColor: theme.palette.success.light,
		height: 6,
		width: 12,
		borderRadius: 3,
	}),
	user: (theme) => ({
		backgroundColor: theme.palette.text.primary,
		height: 8,
		width: 8,
		borderRadius: 4,
		float: "right",
	}),
	body: {
		width: 120,
		margin: "auto",
	},
	title: (theme) => ({
		backgroundColor: theme.palette.text.primary,
		height: 8,
		width: 45,
		borderRadius: 4,
		marginBottom: 6,
	}),
	table: (theme) => ({
		border: `1px solid ${theme.palette.divider}`,
		borderBottom: "none",
		borderTopLeftRadius: 3,
		borderTopRightRadius: 3,
		overflow: "clip",
	}),
	tableHeader: (theme) => ({
		backgroundColor: theme.palette.background.paper,
		height: 10,
		margin: -1,
	}),
	label: (theme) => ({
		display: "flex",
		alignItems: "center",
		justifyContent: "space-between",
		borderTop: `1px solid ${theme.palette.divider}`,
		padding: "4px 12px",
		fontSize: 14,
	}),
	workspace: (theme) => ({
		borderTop: `1px solid ${theme.palette.divider}`,
		height: 15,

		"&::after": {
			content: '""',
			display: "block",
			marginTop: 4,
			marginLeft: 4,
			backgroundColor: theme.palette.text.disabled,
			height: 6,
			width: 30,
			borderRadius: 3,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
