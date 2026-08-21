import type { FC } from "react";
import type {
	DeploymentValues,
	ExternalAuthConfig,
} from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { PremiumPaywallSmall } from "#/modules/paywall/PremiumPaywallSmall";
import { docs } from "#/utils/docs";

type ExternalAuthSettingsPageViewProps = {
	config: DeploymentValues;
	/** True when the deployment may configure more than one provider. */
	isEntitled: boolean;
	canViewPremium: boolean;
};

export const ExternalAuthSettingsPageView: FC<
	ExternalAuthSettingsPageViewProps
> = ({ config, isEntitled, canViewPremium }) => {
	return (
		<>
			<SettingsHeader
				actions={<SettingsHeaderDocsLink href={docs("/admin/external-auth")} />}
			>
				<SettingsHeaderTitle>External Authentication</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Coder integrates with GitHub, GitLab, BitBucket, Azure Repos, and
					OpenID Connect to authenticate developers with external services.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<video
				autoPlay
				muted
				loop
				playsInline
				src="/external-auth.mp4"
				style={{
					maxWidth: "100%",
					borderRadius: 4,
				}}
			/>

			{!isEntitled && (
				<div className="mt-6 mb-6">
					<PremiumPaywallSmall
						source="external_auth"
						message="External Authentication"
						description="Connect multiple Git and OAuth providers at once."
						features={[
							"Connect multiple Git providers at once",
							"Match providers by regex per host",
							"Separate credentials for each provider",
						]}
						canViewPremium={canViewPremium}
					/>
				</div>
			)}

			<Table className="[&_td]:py-6 [&_td:last-child]:pl-8 [&_th:last-child]:pl-8">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">ID</TableHead>
						<TableHead className="w-1/3">Client ID</TableHead>
						<TableHead className="w-1/3">Match</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{config.external_auth === null ||
					config.external_auth?.length === 0 ? (
						<TableEmpty message="No providers have been configured!" />
					) : (
						config.external_auth?.map((git: ExternalAuthConfig) => {
							const name = git.id || git.type;
							return (
								<TableRow key={name}>
									<TableCell>{name}</TableCell>
									<TableCell>{git.client_id}</TableCell>
									<TableCell>{git.regex || "Not Set"}</TableCell>
								</TableRow>
							);
						})
					)}
				</TableBody>
			</Table>
		</>
	);
};
