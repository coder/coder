import { useFormik } from "formik";
import type { FC } from "react";
import type { UpdateAppearanceConfig } from "#/api/typesGenerated";
import {
	Badges,
	EnterpriseBadge,
	PremiumBadge,
} from "#/components/Badges/Badges";
import { Button } from "#/components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	VerticalForm,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { PaywallSmall } from "#/components/Paywall/PaywallSmall";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import type { Permissions } from "#/modules/permissions";
import { getFormHelpers } from "#/utils/formUtils";
import { AnnouncementBannerSettings } from "./AnnouncementBannerSettings";

type AppearanceSettingsPageViewProps = {
	appearance: UpdateAppearanceConfig;
	isEntitled: boolean;
	isPremium: boolean;
	permissions: Permissions;
	onSaveAppearance: (
		newConfig: Partial<UpdateAppearanceConfig>,
	) => Promise<void>;
};

export const AppearanceSettingsPageView: FC<
	AppearanceSettingsPageViewProps
> = ({ appearance, isEntitled, isPremium, permissions, onSaveAppearance }) => {
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
	const fieldsDisabled = !isEntitled || form.isSubmitting;

	return (
		<div className="flex flex-col gap-12">
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>Appearance</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Customize the look and feel of your Coder deployment.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Badges>
					{isEntitled ? (
						isPremium ? (
							<PremiumBadge />
						) : (
							<EnterpriseBadge />
						)
					) : (
						<div className="mb-4">
							<PaywallSmall
								message="Appearance"
								description="With a Premium license, you can customize the appearance and branding of your deployment."
								canViewPremium={permissions.viewAllLicenses}
							/>
						</div>
					)}
				</Badges>

				<VerticalForm
					onSubmit={form.handleSubmit}
					aria-label="Appearance settings"
					className="mt-8"
				>
					<FormSection
						title="Branding"
						description="Customize the application name and logo shown on the login page and in the dashboard."
					>
						<FormFields>
							<FormField
								field={getFieldHelpers("application_name", {
									helperText: isEntitled
										? 'Leave empty to use "Coder".'
										: "This is an Enterprise only feature.",
								})}
								label="Application name"
								placeholder="Coder"
								disabled={fieldsDisabled}
							/>

							<IconField
								{...getFieldHelpers("logo_url", {
									helperText: isEntitled
										? "Leave empty to use the Coder logo. An image with transparency and an aspect ratio of 3:1 or less will look best."
										: "This is an Enterprise only feature.",
								})}
								label="Logo URL"
								placeholder="/icon/coder.svg"
								disabled={fieldsDisabled}
								onPickEmoji={(value) => {
									void form.setFieldValue("logo_url", value);
								}}
							/>
						</FormFields>
					</FormSection>

					<FormFooter>
						<Button type="submit" disabled={fieldsDisabled}>
							<Spinner loading={form.isSubmitting} />
							Save
						</Button>
					</FormFooter>
				</VerticalForm>
			</div>

			<AnnouncementBannerSettings
				isEntitled={isEntitled}
				announcementBanners={appearance.announcement_banners || []}
				onSubmit={(announcementBanners) =>
					onSaveAppearance({ announcement_banners: announcementBanners })
				}
			/>
		</div>
	);
};
