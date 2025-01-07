import InputAdornment from "@mui/material/InputAdornment";
import TextField from "@mui/material/TextField";
import type { UpdateAppearanceConfig } from "api/typesGenerated";
import {
	Badges,
	EnterpriseBadge,
	PremiumBadge,
} from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { useFormik } from "formik";
import type { FC } from "react";
import { getFormHelpers } from "utils/formUtils";
import { Fieldset } from "../Fieldset";
import { AnnouncementBannerSettings } from "./AnnouncementBannerSettings";

export type AppearanceSettingsPageViewProps = {
	appearance: UpdateAppearanceConfig;
	isEntitled: boolean;
	isPremium: boolean;
	onSaveAppearance: (
		newConfig: Partial<UpdateAppearanceConfig>,
	) => Promise<void>;
};

export const AppearanceSettingsPageView: FC<
	AppearanceSettingsPageViewProps
> = ({ appearance, isEntitled, isPremium, onSaveAppearance }) => {
	const applicationNameForm = useFormik<{
		application_name: string;
	}>({
		initialValues: {
			application_name: appearance.application_name,
		},
		onSubmit: (values) => onSaveAppearance(values),
	});
	const applicationNameFieldHelpers = getFormHelpers(applicationNameForm);

	const logoForm = useFormik<{
		logo_url: string;
	}>({
		initialValues: {
			logo_url: appearance.logo_url,
		},
		onSubmit: (values) => onSaveAppearance(values),
	});
	const logoFieldHelpers = getFormHelpers(logoForm);

	return (
		<>
			<SettingsHeader
				title="Appearance"
				description="Customize the look and feel of your Coder deployment."
			/>

			<Badges>
				<Popover mode="hover">
					{isEntitled && !isPremium ? (
						<EnterpriseBadge />
					) : (
						<PopoverTrigger>
							<span>
								<PremiumBadge />
							</span>
						</PopoverTrigger>
					)}

					<PopoverContent css={{ transform: "translateY(-28px)" }}>
						<PopoverPaywall
							message="Appearance"
							description="With a Premium license, you can customize the appearance and branding of your deployment."
							documentationLink="https://coder.com/docs/admin/appearance"
						/>
					</PopoverContent>
				</Popover>
			</Badges>

			<Fieldset
				title="Application name"
				subtitle="Specify a custom application name to be displayed on the login page."
				validation={!isEntitled ? "This is an Enterprise only feature." : ""}
				onSubmit={applicationNameForm.handleSubmit}
				button={!isEntitled && <Button disabled>Submit</Button>}
			>
				<TextField
					{...applicationNameFieldHelpers("application_name")}
					defaultValue={appearance.application_name}
					fullWidth
					placeholder='Leave empty to display "Coder".'
					disabled={!isEntitled}
					inputProps={{
						"aria-label": "Application name",
					}}
				/>
			</Fieldset>

			<Fieldset
				title="Logo URL"
				subtitle="Specify a custom URL for your logo to be displayed on the sign in page and in the top left
          corner of the dashboard."
				validation={
					isEntitled
						? "We recommend a transparent image with 3:1 aspect ratio."
						: "This is an Enterprise only feature."
				}
				onSubmit={logoForm.handleSubmit}
				button={!isEntitled && <Button disabled>Submit</Button>}
			>
				<TextField
					{...logoFieldHelpers("logo_url")}
					defaultValue={appearance.logo_url}
					fullWidth
					placeholder="Leave empty to display the Coder logo."
					disabled={!isEntitled}
					InputProps={{
						endAdornment: (
							<InputAdornment
								position="end"
								css={{
									width: 24,
									height: 24,

									"& img": {
										maxWidth: "100%",
									},
								}}
							>
								<img
									alt=""
									src={logoForm.values.logo_url}
									// This prevent browser to display the ugly error icon if the
									// image path is wrong or user didn't finish typing the url
									onError={(e) => {
										e.currentTarget.style.display = "none";
									}}
									onLoad={(e) => {
										e.currentTarget.style.display = "inline";
									}}
								/>
							</InputAdornment>
						),
					}}
					inputProps={{
						"aria-label": "Logo URL",
					}}
				/>
			</Fieldset>

			<AnnouncementBannerSettings
				isEntitled={isEntitled}
				announcementBanners={appearance.announcement_banners || []}
				onSubmit={(announcementBanners) =>
					onSaveAppearance({ announcement_banners: announcementBanners })
				}
			/>
		</>
	);
};
