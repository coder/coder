import { useEffect, useId, useState } from "react";
import {
	type TerminalFontName,
	TerminalFontNames,
	type UpdateUserAppearanceSettingsRequest,
	type UserAppearanceSettings,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Section } from "#/pages/UserSettingsPage/Section";
import type { ConcreteThemeName } from "#/theme";
import {
	DEFAULT_TERMINAL_FONT,
	DEFAULT_TERMINAL_FONT_SIZE,
	terminalFontLabels,
	terminalFontSizes,
	terminalFonts,
} from "#/theme/constants";
import {
	draftFromState,
	draftToUpdate,
	migrateLegacyPreference,
	switchToSingle,
	type ThemeModeDraft,
} from "#/theme/themeMode";
import { SingleModeSection } from "./SingleModeSection";
import { SyncModeSection } from "./SyncModeSection";
import { TerminalPreview } from "./TerminalPreview";

// Display Geist Mono (the default monospace font) first, then the rest
// alphabetically. TerminalFontNames is auto-generated in alphabetical
// order, so we reorder here for a better UX.
const sortedTerminalFontNames = [
	"geist-mono" as TerminalFontName,
	...TerminalFontNames.filter((name) => name !== "" && name !== "geist-mono"),
];

type AppearanceFormValues = {
	draft: ThemeModeDraft;
	terminalFont: TerminalFontName;
	terminalFontSize: number;
};

type AppearanceThemeMode = ThemeModeDraft["mode"];

type AppearanceFormProps = {
	isUpdating?: boolean;
	error?: unknown;
	initialValues: UserAppearanceSettings;
	activeScheme: "dark" | "light"; // The OS color scheme currently in effect
	onSubmit: (values: UpdateUserAppearanceSettingsRequest) => void;
};

export const AppearanceForm: React.FC<AppearanceFormProps> = ({
	isUpdating,
	error,
	onSubmit,
	initialValues,
	activeScheme,
}) => {
	const [values, setValues] = useState(() => toFormValues(initialValues));
	const themeModeId = useId();
	const fontGroupId = useId();
	const fontFamilyId = `${fontGroupId}-family`;
	const fontSizeId = `${fontGroupId}-size`;
	const singleThemeGroupName = `${themeModeId}-single`;
	const syncThemeGroupNamePrefix = `${themeModeId}-sync`;

	const {
		theme_preference,
		theme_mode,
		theme_light,
		theme_dark,
		terminal_font,
		terminal_font_size,
	} = initialValues;

	useEffect(() => {
		if (isUpdating) {
			return;
		}
		const next = toFormValues({
			theme_preference,
			theme_mode,
			theme_light,
			theme_dark,
			terminal_font,
			terminal_font_size,
		});
		setValues(next);
	}, [
		isUpdating,
		theme_preference,
		theme_mode,
		theme_light,
		theme_dark,
		terminal_font,
		terminal_font_size,
	]);

	const { draft, terminalFont, terminalFontSize } = values;

	const submit = (next: AppearanceFormValues) => {
		setValues(next);
		onSubmit(toUpdateRequest(next, activeScheme));
	};

	const onChangeMode = (mode: AppearanceThemeMode) => {
		if (mode === draft.mode) {
			return;
		}
		const next: ThemeModeDraft =
			mode === "single"
				? {
						mode: "single",
						single: switchToSingle(
							{ mode: "sync", light: draft.light, dark: draft.dark },
							activeScheme,
						).theme,
						light: draft.light,
						dark: draft.dark,
					}
				: {
						mode: "sync",
						single: draft.single,
						light: draft.light,
						dark: draft.dark,
					};
		submit({ ...values, draft: next });
	};

	const onSelectSyncSlot = (
		scheme: "light" | "dark",
		theme: ConcreteThemeName,
	) => {
		const next: ThemeModeDraft =
			scheme === "light"
				? { ...draft, light: theme }
				: { ...draft, dark: theme };
		submit({ ...values, draft: next });
	};

	const onSelectSingle = (theme: ConcreteThemeName) => {
		submit({
			...values,
			draft: { ...draft, single: theme, mode: "single" },
		});
	};

	const onChangeTerminalFont = (nextTerminalFont: TerminalFontName) => {
		submit({ ...values, terminalFont: nextTerminalFont });
	};

	const onChangeTerminalFontSize = (nextTerminalFontSize: number) => {
		submit({ ...values, terminalFontSize: nextTerminalFontSize });
	};

	return (
		<form onSubmit={(event) => event.preventDefault()}>
			{Boolean(error) && <ErrorAlert error={error} />}

			<Section
				title={
					<div className="flex items-center gap-2">
						<span>Theme</span>
						<Spinner
							loading={isUpdating}
							size="sm"
							label="Saving theme preference"
						/>
					</div>
				}
				layout="fluid"
				className="mb-12"
			>
				<div className="flex flex-col gap-4">
					<div className="flex flex-col gap-2">
						<Label htmlFor={themeModeId} className="text-sm font-medium">
							Theme mode
						</Label>
						<div className="flex items-center gap-4">
							<Select
								value={draft.mode}
								onValueChange={(value) => {
									if (isThemeMode(value)) {
										onChangeMode(value);
									}
								}}
							>
								<SelectTrigger
									id={themeModeId}
									className="w-48 text-content-primary"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="sync">Sync with system</SelectItem>
									<SelectItem value="single">Single theme</SelectItem>
								</SelectContent>
							</Select>
						</div>
					</div>

					{draft.mode === "sync" ? (
						<SyncModeSection
							light={draft.light}
							dark={draft.dark}
							activeScheme={activeScheme}
							namePrefix={syncThemeGroupNamePrefix}
							onSelect={onSelectSyncSlot}
						/>
					) : (
						<SingleModeSection
							selected={draft.single}
							name={singleThemeGroupName}
							onSelect={onSelectSingle}
						/>
					)}
				</div>
			</Section>

			<Section
				title={
					<div className="flex items-center gap-2">
						<span>Terminal</span>
						<Spinner
							loading={isUpdating}
							size="sm"
							label="Saving terminal font"
						/>
					</div>
				}
				layout="fluid"
			>
				<div className="flex flex-col gap-6">
					<div className="flex gap-4">
						<div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-none">
							<Label htmlFor={fontFamilyId} className="text-sm font-medium">
								Font
							</Label>
							<Select
								value={terminalFont}
								onValueChange={(value) =>
									onChangeTerminalFont(toTerminalFontName(value))
								}
							>
								<SelectTrigger
									id={fontFamilyId}
									className="w-full text-content-primary sm:w-48"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{sortedTerminalFontNames.map((name) => (
										<SelectItem
											key={name}
											value={name}
											style={{ fontFamily: terminalFonts[name] }}
										>
											{terminalFontLabels[name]}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-none">
							<Label htmlFor={fontSizeId} className="text-sm font-medium">
								Font size
							</Label>
							<Select
								value={String(terminalFontSize)}
								onValueChange={(value) =>
									onChangeTerminalFontSize(Number(value))
								}
							>
								<SelectTrigger
									id={fontSizeId}
									className="w-full text-content-primary sm:w-48"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{terminalFontSizes.map((size) => (
										<SelectItem key={size} value={String(size)}>
											{size}px
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					</div>

					<TerminalPreview
						fontFamily={terminalFonts[terminalFont]}
						fontSize={terminalFontSize}
					/>
				</div>
			</Section>
		</form>
	);
};

function toFormValues(settings: UserAppearanceSettings): AppearanceFormValues {
	return {
		draft: draftFromState(migrateLegacyPreference(settings), {
			light: settings.theme_light,
			dark: settings.theme_dark,
		}),
		terminalFont: settings.terminal_font || DEFAULT_TERMINAL_FONT,
		terminalFontSize: settings.terminal_font_size || DEFAULT_TERMINAL_FONT_SIZE,
	};
}

function toUpdateRequest(
	values: AppearanceFormValues,
	activeScheme: "dark" | "light",
): UpdateUserAppearanceSettingsRequest {
	return {
		...draftToUpdate(values.draft, values.terminalFont, activeScheme),
		terminal_font_size: values.terminalFontSize,
	};
}

function isThemeMode(value: string): value is AppearanceThemeMode {
	return value === "sync" || value === "single";
}

function toTerminalFontName(value: string): TerminalFontName {
	return TerminalFontNames.includes(value as TerminalFontName)
		? (value as TerminalFontName)
		: "";
}
