import { useFormik } from "formik";
import type { FC } from "react";
import type { UpdateAppearanceConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	VerticalForm,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { getFormHelpers } from "#/utils/formUtils";
import { AnnouncementBannerSettings } from "./AnnouncementBannerSettings";

type AppearanceSettingsPageViewProps = {
	appearance: UpdateAppearanceConfig;
	isEntitled: boolean;
	canViewPremium: boolean;
	onSaveAppearance: (
		newConfig: Partial<UpdateAppearanceConfig>,
	) => Promise<void>;
};

export const AppearanceSettingsPageView: FC<
	AppearanceSettingsPageViewProps
> = ({ appearance, isEntitled, canViewPremium, onSaveAppearance }) => {
	const form = useFormik<{
		application_name: string;
		logo_url: string;
	}>({
		initialValues: {
			application_name: appearance.application_name,
			logo_url: appearance.logo_url,
		},
		onSubmit: (values) => onSaveAppearance(values),
		enableReinitialize: true,
	});
	const getFieldHelpers = getFormHelpers(form);

	return (
		<div className="flex flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Appearance</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Customize the look and feel of your Coder deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{!isEntitled ? (
				<PaywallPremium
					message="Appearance"
					description="With a Premium license, you can customize branding and announcement banners for your deployment."
					canViewPremium={canViewPremium}
				/>
			) : (
				<>
					<VerticalForm
						onSubmit={form.handleSubmit}
						aria-label="Appearance settings"
					>
						<FormSection
							title="Branding"
							description="Customize the application name and logo shown on the login page and in the dashboard."
						>
							<FormFields>
								<FormField
									field={getFieldHelpers("application_name", {
										helperText: 'Leave empty to use "Coder".',
									})}
									label="Application name"
									placeholder="Coder"
									disabled={form.isSubmitting}
								/>

								<IconField
									{...getFieldHelpers("logo_url", {
										helperText:
											"Leave empty to use the Coder logo. An image with transparency and an aspect ratio of 3:1 or less will look best.",
									})}
									label="Logo URL"
									placeholder="/icon/coder.svg"
									disabled={form.isSubmitting}
									onPickEmoji={(value) => {
										void form.setFieldValue("logo_url", value);
									}}
								/>
							</FormFields>
						</FormSection>

						<FormFooter>
							<Button type="submit" disabled={form.isSubmitting}>
								<Spinner loading={form.isSubmitting} />
								Save
							</Button>
						</FormFooter>
					</VerticalForm>

					<AnnouncementBannerSettings
						isEntitled
						announcementBanners={appearance.announcement_banners || []}
						onSubmit={(announcementBanners) =>
							onSaveAppearance({ announcement_banners: announcementBanners })
						}
					/>
				</>
			)}

			<div className="overflow-hidden rounded-lg border border-solid border-border">
				<div className="flex items-center justify-between gap-4 p-6">
					<div>
						<h3 className="m-0 text-xl font-semibold">
							<label htmlFor="hide-codernauts">Hide Codernauts game</label>
						</h3>
						<div className="mt-2 text-sm text-content-secondary">
							Remove the Codernauts game link from the user dropdown menu.
							Available on all licenses.
						</div>
					</div>
					<Switch
						id="hide-codernauts"
						checked={appearance.hide_codernauts}
						onCheckedChange={(checked) =>
							onSaveAppearance({ hide_codernauts: checked })
						}
					/>
				</div>
			</div>
		</div>
	);
};
