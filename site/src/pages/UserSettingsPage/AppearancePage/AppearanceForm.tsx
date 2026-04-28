import { type FC, useMemo, useRef, useState } from "react";
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
import type { ConcreteThemeName } from "#/theme/colorblind";
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
import { Section } from "../Section";
import { SingleModeSection } from "./SingleModeSection";
import { SyncModeSection } from "./SyncModeSection";

// Display Geist Mono (the default monospace font) first, then the rest
// alphabetically. TerminalFontNames is auto-generated in alphabetical
// order, so we reorder here for a better UX.
const sortedTerminalFontNames = [
	"geist-mono" as TerminalFontName,
	...TerminalFontNames.filter((name) => name !== "" && name !== "geist-mono"),
];

interface AppearanceFormProps {
	isUpdating?: boolean;
	error?: unknown;
	/**
	 * The full appearance settings document from the API. Unlike the
	 * previous shape this accepts every field including legacy auto-*
	 * values so the migration helper can classify them on mount.
	 */
	initialValues: UserAppearanceSettings;
	/**
	 * The OS color scheme currently in effect. Used to decide which
	 * sync card shows the "Active" pill.
	 */
	activeScheme: "dark" | "light";
	onSubmit: (values: UpdateUserAppearanceSettingsRequest) => Promise<unknown>;
}

export const AppearanceForm: FC<AppearanceFormProps> = ({
	isUpdating,
	error,
	onSubmit,
	initialValues,
	activeScheme,
}) => {
	// Seed the working draft once from the persisted settings. The
	// draft carries all four slots (mode, single, light, dark) so the
	// user can switch the dropdown without losing their other-mode
	// selection mid-interaction.
	const [draft, setDraft] = useState<ThemeModeDraft>(() =>
		draftFromState(migrateLegacyPreference(initialValues), {
			light: initialValues.theme_light,
			dark: initialValues.theme_dark,
		}),
	);

	const currentTerminalFont = useMemo(
		() => initialValues.terminal_font || DEFAULT_TERMINAL_FONT,
		[initialValues.terminal_font],
	);
	const submitInFlightRef = useRef(false);

	const submit = (next: ThemeModeDraft, terminalFont: TerminalFontName) => {
		if (isUpdating || submitInFlightRef.current) {
			return;
		}
		submitInFlightRef.current = true;
		setDraft(next);
		try {
			const submitted = onSubmit(draftToUpdate(next, terminalFont));
			const resetSubmitInFlight = () => {
				submitInFlightRef.current = false;
			};
			void submitted.then(resetSubmitInFlight, resetSubmitInFlight);
			return submitted;
		} catch (error) {
			submitInFlightRef.current = false;
			throw error;
		}
	};

	const onChangeMode = (mode: "sync" | "single") => {
		if (mode === draft.mode) {
			return;
		}
		// Preserve every slot the user has already picked across the
		// toggle. Switching sync <-> single is a reversible UI choice;
		// the user's sync pair must survive a detour through single
		// mode, and vice versa.
		const next: ThemeModeDraft =
			mode === "single"
				? {
						mode: "single",
						// switchToSingle picks the slot that matches
						// the active OS scheme so the rendered theme
						// does not flip when the dropdown changes.
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
		submit(next, currentTerminalFont);
	};

	const onSelectSyncSlot = (
		scheme: "light" | "dark",
		theme: ConcreteThemeName,
	) => {
		submit(
			{ ...draft, [scheme]: theme } as ThemeModeDraft,
			currentTerminalFont,
		);
	};

	const onSelectSingle = (theme: ConcreteThemeName) => {
		submit({ ...draft, single: theme, mode: "single" }, currentTerminalFont);
	};

	const onChangeTerminalFont = (terminalFont: TerminalFontName) => {
		submit(draft, terminalFont);
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
				<div className="flex flex-col gap-4">
					<div className="flex flex-col gap-2">
						<Label htmlFor="theme-mode" className="text-sm font-medium">
							Theme mode
						</Label>
						<div className="flex items-center gap-4">
							<Select
								value={draft.mode}
								onValueChange={(value) =>
									onChangeMode(value as "sync" | "single")
								}
							>
								<SelectTrigger
									id="theme-mode"
									className="w-48 text-content-primary"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="sync">Sync with system</SelectItem>
									<SelectItem value="single">Single theme</SelectItem>
								</SelectContent>
							</Select>
							<span className="text-sm text-content-secondary">
								{draft.mode === "sync"
									? "Coder theme will match your system active settings"
									: "Coder will use your selected theme"}
							</span>
						</div>
					</div>

					{draft.mode === "sync" ? (
						<SyncModeSection
							light={draft.light}
							dark={draft.dark}
							activeScheme={activeScheme}
							onSelect={onSelectSyncSlot}
						/>
					) : (
						<SingleModeSection
							selected={draft.single}
							onSelect={onSelectSingle}
						/>
					)}
				</div>
			</Section>

			<Section
				title={
					<div className="flex flex-row items-center gap-2">
						<span id="fonts-radio-buttons-group-label">Terminal Font</span>
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
					{sortedTerminalFontNames.map((name) => (
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
