import { type FC, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	appearanceSettings,
	updateAppearanceSettings,
} from "#/api/queries/users";
import type { UpdateUserAppearanceSettingsRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	isCoderAssistantHidden,
	setCoderAssistantHidden,
} from "#/components/CoderAssistant/visibility";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import { Switch } from "#/components/Switch/Switch";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { Section } from "#/pages/UserSettingsPage/Section";
import { usePreferredColorScheme } from "#/theme/usePreferredColorScheme";
import { AppearanceForm } from "./AppearanceForm";

type MutateAppearanceSettings = (
	values: UpdateUserAppearanceSettingsRequest,
	options: { onSettled: () => void },
) => void;

export const useQueuedAppearanceSubmit = (mutate: MutateAppearanceSettings) => {
	const submitInFlightRef = useRef(false);
	const pendingSubmitRef = useRef<UpdateUserAppearanceSettingsRequest | null>(
		null,
	);

	const fireSubmit = (values: UpdateUserAppearanceSettingsRequest) => {
		submitInFlightRef.current = true;
		mutate(values, {
			onSettled: () => {
				const queued = pendingSubmitRef.current;
				pendingSubmitRef.current = null;
				if (queued !== null) {
					fireSubmit(queued);
					return;
				}
				submitInFlightRef.current = false;
			},
		});
	};

	return (values: UpdateUserAppearanceSettingsRequest) => {
		if (submitInFlightRef.current) {
			pendingSubmitRef.current = values;
			return;
		}

		fireSubmit(values);
	};
};

// The Coder Assistant is enabled per deployment (coder-assistant
// experiment) and shown to everyone by default. This is a per-browser
// preference to hide it. The provider reads it once at mount, so
// changes take effect after a page reload.
const CoderAssistantSection: FC = () => {
	const switchId = useId();
	const [assistantVisible, setAssistantVisible] = useState(
		() => !isCoderAssistantHidden(),
	);

	const handleToggle = (checked: boolean) => {
		setAssistantVisible(checked);
		setCoderAssistantHidden(!checked);
		if (checked) {
			try {
				// Re-showing from settings implies the user is past the
				// intro, so skip the onboarding flow on future visits.
				localStorage.setItem("coder_assistant_intro_completed", "true");
			} catch {
				// Storage may be unavailable in some contexts.
			}
		}
	};

	return (
		<Section title="Coder Assistant" layout="fluid" className="mt-12">
			<div className="flex items-center gap-3">
				<Switch
					id={switchId}
					checked={assistantVisible}
					onCheckedChange={handleToggle}
				/>
				<div className="flex flex-col">
					<Label htmlFor={switchId} className="cursor-pointer font-normal">
						Show the Coder Assistant
					</Label>
					<span className="text-xs text-content-secondary">
						Display the floating assistant button in the dashboard. Takes effect
						after a page reload.
					</span>
				</div>
			</div>
		</Section>
	);
};

const AppearancePage: FC = () => {
	const queryClient = useQueryClient();
	const { experiments } = useDashboard();
	const coderAssistantEnabled = experiments.includes("coder-assistant");
	const updateAppearanceSettingsMutation = useMutation(
		updateAppearanceSettings(queryClient),
	);

	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const osColorScheme = usePreferredColorScheme();
	const submitAppearanceSettings = useQueuedAppearanceSubmit(
		(values, options) => {
			updateAppearanceSettingsMutation.mutate(values, {
				onSettled: options.onSettled,
			});
		},
	);

	if (appearanceSettingsQuery.isLoading) {
		return <Loader />;
	}

	if (!appearanceSettingsQuery.data) {
		return <ErrorAlert error={appearanceSettingsQuery.error} />;
	}

	return (
		<>
			<AppearanceForm
				isUpdating={updateAppearanceSettingsMutation.isPending}
				error={updateAppearanceSettingsMutation.error}
				initialValues={appearanceSettingsQuery.data}
				activeScheme={osColorScheme}
				onSubmit={submitAppearanceSettings}
			/>
			{coderAssistantEnabled && <CoderAssistantSection />}
		</>
	);
};

export default AppearancePage;
