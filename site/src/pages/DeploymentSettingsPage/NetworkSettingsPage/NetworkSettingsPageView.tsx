import { Link as RouterLink } from "react-router";
import type { SerpentOption } from "#/api/typesGenerated";
import { BadgeGroup } from "#/components/Badge/Badge";
import {
	DisabledBadge,
	EnabledBadge,
	PremiumBadge,
} from "#/components/Badge/PresetBadges";
import { Link } from "#/components/Link/Link";
import { PREMIUM_PAGE_PATH } from "#/components/Paywall/Paywall";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	deploymentGroupHasParent,
	useDeploymentOptions,
} from "#/utils/deployOptions";
import { docs } from "#/utils/docs";
import OptionsTable from "../OptionsTable";

type NetworkSettingsPageViewProps = {
	options: SerpentOption[];
	/** When false, options that need a license move to their own Premium section. */
	hasLicense: boolean;
	/** Whether the user can open the Premium page linked from the Premium section. */
	canViewPremium: boolean;
};

export const NetworkSettingsPageView: React.FC<
	NetworkSettingsPageViewProps
> = ({ options, hasLicense, canViewPremium }) => {
	const networkingOptions = options.filter((o) =>
		deploymentGroupHasParent(o.group, "Networking"),
	);
	const premiumOptions = hasLicense
		? []
		: networkingOptions.filter((o) => o.annotations?.enterprise === "true");
	const nonPremiumOptions = networkingOptions.filter(
		(o) => !premiumOptions.includes(o),
	);

	return (
		<div className="flex flex-col gap-12">
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>Network</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Configure your deployment connectivity.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/networking")}
							context="about deployment networking"
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<OptionsTable
					options={nonPremiumOptions}
					aria-label="Network options"
				/>
			</div>

			{premiumOptions.length > 0 && (
				<div>
					<SettingsHeader>
						<SettingsHeaderTitle level="h2" hierarchy="secondary">
							Premium settings
							<BadgeGroup>
								<PremiumBadge />
							</BadgeGroup>
						</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							These settings require a Premium license to take effect.
							{canViewPremium && (
								<>
									{" "}
									<Link asChild showExternalIcon={false}>
										<RouterLink to={PREMIUM_PAGE_PATH}>
											Learn more about Premium
										</RouterLink>
									</Link>
								</>
							)}
						</SettingsHeaderDescription>
					</SettingsHeader>

					<OptionsTable
						options={premiumOptions}
						aria-label="Premium network options"
					/>
				</div>
			)}

			<div>
				<SettingsHeader>
					<SettingsHeaderTitle level="h2" hierarchy="secondary">
						Port Forwarding
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Port forwarding lets developers securely access processes on their
						Coder workspace from a local machine.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/networking/port-forwarding")}
							context="about port forwarding"
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<BadgeGroup>
					{useDeploymentOptions(options, "Wildcard Access URL")[0].value !==
					"" ? (
						<EnabledBadge />
					) : (
						<DisabledBadge />
					)}
				</BadgeGroup>
			</div>
		</div>
	);
};
