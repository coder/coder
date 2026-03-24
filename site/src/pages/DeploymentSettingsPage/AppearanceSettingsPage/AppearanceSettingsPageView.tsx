import type { UpdateAppearanceConfig } from "api/typesGenerated";
import {
	Badges,
	EnterpriseBadge,
	PremiumBadge,
} from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "components/InputGroup/InputGroup";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useFormik } from "formik";
import type { FC } from "react";
import { getFormHelpers } from "utils/formUtils";
import { Fieldset } from "../Fieldset";
import { AnnouncementBannerSettings } from "./AnnouncementBannerSettings";

type AppearanceSettingsPageViewProps = {
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
			<SettingsHeader>
				<SettingsHeaderTitle>Appearance</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Customize the look and feel of your Coder deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<Badges>
				<Tooltip>
					{isEntitled && !isPremium ? (
						<EnterpriseBadge />
					) : (
						<TooltipTrigger asChild>
							<span>
								<PremiumBadge />
							</span>
						</TooltipTrigger>
					)}

					<TooltipContent
						sideOffset={-28}
						collisionPadding={16}
						className="p-0"
					>
						<PopoverPaywall
							message="Appearance"
							description="With a Premium license, you can customize the appearance and branding of your deployment."
							documentationLink="https://coder.com/docs/admin/appearance"
						/>
					</TooltipContent>
				</Tooltip>
			</Badges>

			<Fieldset
				title="Application name"
				subtitle="Specify a custom application name to be displayed on the login page."
				validation={!isEntitled ? "This is an Enterprise only feature." : ""}
				onSubmit={applicationNameForm.handleSubmit}
				button={!isEntitled && <Button disabled>Submit</Button>}
			>
				<Input
					{...applicationNameFieldHelpers("application_name")}
					placeholder='Leave empty to display "Coder".'
					disabled={!isEntitled}
					aria-label="Application name"
				/>
			</Fieldset>

			<Fieldset
				title="Logo URL"
				subtitle="Specify a custom URL for your logo to be displayed on the sign in page and in the top left
          corner of the dashboard."
				validation={
					isEntitled
						? "An image with transparency and an aspect ratio of 3:1 or less will look best."
						: "This is an Enterprise only feature."
				}
				onSubmit={logoForm.handleSubmit}
				button={!isEntitled && <Button disabled>Submit</Button>}
			>
				<InputGroup>
					<InputGroupInput
						{...logoFieldHelpers("logo_url")}
						placeholder="Leave empty to display the Coder logo."
						disabled={!isEntitled}
						aria-label="Logo URL"
					/>
					<InputGroupAddon align="inline-end">
						<img
							alt=""
							src={logoForm.values.logo_url}
							className="h-6 w-6 max-w-full object-contain"
							// Hide broken image icon while users type incomplete URLs.
							onError={(e) => {
								e.currentTarget.style.display = "none";
							}}
							onLoad={(e) => {
								e.currentTarget.style.display = "inline";
							}}
						/>
					</InputGroupAddon>
				</InputGroup>
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
