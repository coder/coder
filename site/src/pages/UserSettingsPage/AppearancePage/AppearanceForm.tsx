import { type FC, useEffect, useId, useState } from "react";
import {
	type TerminalFontName,
	TerminalFontNames,
	type UpdateUserAppearanceSettingsRequest,
	type UserAppearanceSettings,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
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
	terminalFontLabels,
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

// Display Geist Mono (the default monospace font) first, then the rest
// alphabetically. TerminalFontNames is auto-generated in alphabetical
// order, so we reorder here for a better UX.
const sortedTerminalFontNames = [
	"geist-mono" as TerminalFontName,
	...TerminalFontNames.filter((name) => name !== "" && name !== "geist-mono"),
];

interface AppearanceFormValues {
	draft: ThemeModeDraft;
	terminalFont: TerminalFontName;
}

type AppearanceThemeMode = ThemeModeDraft["mode"];

interface AppearanceFormProps {
	isUpdating?: boolean;
	error?: unknown;
	initialValues: UserAppearanceSettings;
	activeScheme: "dark" | "light"; // The OS color scheme currently in effect
	onSubmit: (values: UpdateUserAppearanceSettingsRequest) => void;
}

export const AppearanceForm: FC<AppearanceFormProps> = ({
	isUpdating,
	error,
	onSubmit,
	initialValues,
	activeScheme,
}) => {
	const [values, setValues] = useState(() => toFormValues(initialValues));
	const themeModeId = useId();
	const fontGroupId = useId();
	const fontGroupLabelId = `${fontGroupId}-label`;
	const fontGroupName = `${fontGroupId}-fonts`;
	const singleThemeGroupName = `${themeModeId}-single`;
	const syncThemeGroupNamePrefix = `${themeModeId}-sync`;

	const {
		theme_preference,
		theme_mode,
		theme_light,
		theme_dark,
		terminal_font,
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
		});
		setValues(next);
	}, [
		isUpdating,
		theme_preference,
		theme_mode,
		theme_light,
		theme_dark,
		terminal_font,
	]);

	const { draft, terminalFont } = values;

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
		submit({ draft: next, terminalFont });
	};

	const onSelectSyncSlot = (
		scheme: "light" | "dark",
		theme: ConcreteThemeName,
	) => {
		const next: ThemeModeDraft =
			scheme === "light"
				? { ...draft, light: theme }
				: { ...draft, dark: theme };
		submit({ draft: next, terminalFont });
	};

	const onSelectSingle = (theme: ConcreteThemeName) => {
		submit({
			draft: { ...draft, single: theme, mode: "single" },
			terminalFont,
		});
	};

	const onChangeTerminalFont = (nextTerminalFont: TerminalFontName) => {
		submit({ draft, terminalFont: nextTerminalFont });
	};

	return (
		<form onSubmit={(event) => event.preventDefault()}>
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
					<div className="flex flex-row items-center gap-2">
						<span id={fontGroupLabelId}>Terminal Font</span>
						<Spinner loading={isUpdating} size="sm" />
					</div>
				}
				layout="fluid"
			>
				<RadioGroup
					aria-labelledby={fontGroupLabelId}
					value={terminalFont}
					name={fontGroupName}
					onValueChange={(value) =>
						onChangeTerminalFont(toTerminalFontName(value))
					}
				>
					{sortedTerminalFontNames.map((name) => (
						<div key={name} className="flex items-center space-x-2">
							<RadioGroupItem value={name} id={`${fontGroupId}-${name}`} />
							<Label
								htmlFor={`${fontGroupId}-${name}`}
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

function toFormValues(settings: UserAppearanceSettings): AppearanceFormValues {
	return {
		draft: draftFromState(migrateLegacyPreference(settings), {
			light: settings.theme_light,
			dark: settings.theme_dark,
		}),
		terminalFont: settings.terminal_font || DEFAULT_TERMINAL_FONT,
	};
}

function toUpdateRequest(
	values: AppearanceFormValues,
	activeScheme: "dark" | "light",
): UpdateUserAppearanceSettingsRequest {
	return draftToUpdate(values.draft, values.terminalFont, activeScheme);
}

function isThemeMode(value: string): value is AppearanceThemeMode {
	return value === "sync" || value === "single";
}

function toTerminalFontName(value: string): TerminalFontName {
	return TerminalFontNames.includes(value as TerminalFontName)
		? (value as TerminalFontName)
		: "";
}
