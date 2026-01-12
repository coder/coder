import {
	type TerminalFontName,
	TerminalFontNames,
	type UpdateUserAppearanceSettingsRequest,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { PreviewBadge } from "components/Badges/Badges";
import { Label } from "components/Label/Label";
import { RadioGroup, RadioGroupItem } from "components/RadioGroup/RadioGroup";
import { Spinner } from "components/Spinner/Spinner";
import type { FC } from "react";
import { DEFAULT_THEME } from "theme";
import {
	DEFAULT_TERMINAL_FONT,
	terminalFontLabels,
	terminalFonts,
} from "theme/constants";
import { cn } from "utils/cn";
import { Section } from "../Section";

interface AppearanceFormProps {
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

	const onChangeTheme = async (theme: string) => {
		if (isUpdating) {
			return;
		}
		await onSubmit({
			theme_preference: theme,
			terminal_font: currentTerminalFont,
		});
	};

	const onChangeTerminalFont = async (terminalFont: TerminalFontName) => {
		if (isUpdating) {
			return;
		}
		await onSubmit({
			theme_preference: currentTheme,
			terminal_font: terminalFont,
		});
	};

	return (
		<form>
			{Boolean(error) && <ErrorAlert error={error} />}

			<Section
				title={
					<div className="flex flex-row items-center gap-2">
						<span>Theme</span>
						<Spinner loading={isUpdating} size="sm" />
					</div>
				}
				layout="fluid"
				className="mb-12"
			>
				<div className="flex flex-row flex-wrap gap-4">
					<AutoThemePreviewButton
						displayName="Auto"
						active={currentTheme === "auto"}
						themes={["dark", "light"]}
						onSelect={() => onChangeTheme("auto")}
					/>
					<ThemePreviewButton
						displayName="Dark"
						active={currentTheme === "dark"}
						theme="dark"
						onSelect={() => onChangeTheme("dark")}
					/>
					<ThemePreviewButton
						displayName="Light"
						active={currentTheme === "light"}
						theme="light"
						onSelect={() => onChangeTheme("light")}
					/>
				</div>
			</Section>
			<Section
				title={
					<div className="flex flex-row items-center gap-2">
						<span id="fonts-radio-buttons-group-label">Terminal font</span>
						<Spinner loading={isUpdating} size="sm" />
					</div>
				}
				layout="fluid"
			>
				<RadioGroup
					aria-labelledby="fonts-radio-buttons-group-label"
					defaultValue={currentTerminalFont}
					name="fonts-radio-buttons-group"
					onValueChange={(value) =>
						onChangeTerminalFont(toTerminalFontName(value))
					}
				>
					{TerminalFontNames.filter((name) => name !== "").map((name) => (
						<div key={name} className="flex items-center space-x-2">
							<RadioGroupItem value={name} id={name} />
							<Label
								htmlFor={name}
								className="cursor-pointer font-normal"
								style={{ fontFamily: terminalFonts[name] }}
							>
								{terminalFontLabels[name]}
							</Label>
						</div>
					))}
				</RadioGroup>
			</Section>
		</form>
	);
};

function toTerminalFontName(value: string): TerminalFontName {
	return TerminalFontNames.includes(value as TerminalFontName)
		? (value as TerminalFontName)
		: "";
}

type ThemeMode = "dark" | "light";

interface AutoThemePreviewButtonProps extends Omit<ThemePreviewProps, "theme"> {
	themes: [ThemeMode, ThemeMode];
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
				className="sr-only"
			/>
			<label
				htmlFor={displayName}
				className={cn("relative cursor-pointer", className)}
			>
				<ThemePreview
					className="absolute"
					style={{
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
				className="sr-only"
			/>
			<label htmlFor={displayName} className={cn("cursor-pointer", className)}>
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
	style?: React.CSSProperties;
	displayName: string;
	theme: ThemeMode;
}

const ThemePreview: FC<ThemePreviewProps> = ({
	active,
	preview,
	className,
	style,
	displayName,
	theme,
}) => {
	return (
		<div className={theme}>
			<div
				className={cn(
					"w-56 overflow-clip rounded-md border border-border border-solid bg-surface-primary text-content-primary select-none",
					active && "outline outline-2 outline-content-link",
					className,
				)}
				style={style}
			>
				<div className="bg-surface-primary text-content-primary">
					<div className="bg-surface-secondary flex items-center justify-between px-2.5 py-1.5 mb-2 border-0 border-b border-border border-solid">
						<div className="flex items-center gap-1.5">
							<div className="bg-content-primary h-1.5 w-5 rounded" />
							<div className="bg-content-secondary h-1.5 w-5 rounded" />
							<div className="bg-content-secondary h-1.5 w-5 rounded" />
						</div>
						<div className="flex items-center gap-1.5">
							<div className="bg-green-400 h-1.5 w-3 rounded" />
							<div className="bg-content-primary h-2 w-2 rounded-full" />
						</div>
					</div>
					<div className="w-32 mx-auto">
						<div className="bg-content-primary h-2 w-11 rounded mb-1.5" />
						<div className="border border-solid rounded-t overflow-clip">
							<div className="bg-surface-secondary h-2.5 -m-px" />
							<div className="h-4 border-0 border-t border-border border-solid">
								<div className="bg-content-disabled h-1.5 w-8 rounded mt-1 ml-1" />
							</div>
							<div className="h-4 border-0 border-t border-border border-solid">
								<div className="bg-content-disabled h-1.5 w-8 rounded mt-1 ml-1" />
							</div>
							<div className="h-4 border-0 border-t border-border border-solid">
								<div className="bg-content-disabled h-1.5 w-8 rounded mt-1 ml-1" />
							</div>
							<div className="h-4 border-0 border-t border-border border-solid">
								<div className="bg-content-disabled h-1.5 w-8 rounded mt-1 ml-1" />
							</div>
						</div>
					</div>
				</div>
				<div className="flex items-center justify-between border-0 border-t border-border border-solid px-3 py-1 text-sm">
					<span>{displayName}</span>
					{preview && <PreviewBadge />}
				</div>
			</div>
		</div>
	);
};
