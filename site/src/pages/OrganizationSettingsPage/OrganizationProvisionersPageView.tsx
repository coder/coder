import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import type {
	BuildInfoResponse,
	ProvisionerKey,
	ProvisionerKeyDaemons,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { Paywall } from "components/Paywall/Paywall";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { ProvisionerGroup } from "modules/provisioners/ProvisionerGroup";
import type { FC } from "react";
import { docs } from "utils/docs";

interface OrganizationProvisionersPageViewProps {
	/** Determines if the paywall will be shown or not */
	showPaywall?: boolean;

	/** An error to display instead of the page content */
	error?: unknown;

	/** Info about the version of coderd */
	buildInfo?: BuildInfoResponse;

	/** Groups of provisioners, along with their key information */
	provisioners?: readonly ProvisionerKeyDaemons[];
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ showPaywall, error, buildInfo, provisioners }) => {
	return (
		<div>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader>
					<SettingsHeaderTitle>Provisioners</SettingsHeaderTitle>
				</SettingsHeader>

				{!showPaywall && (
					<Button
						endIcon={<OpenInNewIcon />}
						target="_blank"
						href={docs("/admin/provisioners")}
					>
						Create a provisioner
					</Button>
				)}
			</Stack>
			{showPaywall ? (
				<Paywall
					message="Provisioners"
					description="Provisioners run your Terraform to create templates and workspaces. You need a Premium license to use this feature for multiple organizations."
					documentationLink={docs("/")}
				/>
			) : error ? (
				<ErrorAlert error={error} />
			) : !buildInfo || !provisioners ? (
				<Loader />
			) : (
				<ViewContent buildInfo={buildInfo} provisioners={provisioners} />
			)}
		</div>
	);
};

type ViewContentProps = Required<
	Pick<OrganizationProvisionersPageViewProps, "buildInfo" | "provisioners">
>;

const ViewContent: FC<ViewContentProps> = ({ buildInfo, provisioners }) => {
	const isEmpty = provisioners.every((group) => group.daemons.length === 0);

	const provisionerGroupsCount = provisioners.length;
	const provisionersCount = provisioners.reduce(
		(a, group) => a + group.daemons.length,
		0,
	);

	return (
		<>
			{isEmpty ? (
				<EmptyState
					message="No provisioners"
					description="A provisioner is required before you can create templates and workspaces. You can connect your first provisioner by following our documentation."
					cta={
						<Button
							endIcon={<OpenInNewIcon />}
							target="_blank"
							href={docs("/admin/provisioners")}
						>
							Create a provisioner
						</Button>
					}
				/>
			) : (
				<div
					css={(theme) => ({
						margin: 0,
						fontSize: 12,
						paddingBottom: 18,
						color: theme.palette.text.secondary,
					})}
				>
					Showing {provisionerGroupsCount} groups and {provisionersCount}{" "}
					provisioners
				</div>
			)}
			<Stack spacing={4.5}>
				{provisioners.map((group) => (
					<ProvisionerGroup
						key={group.key.id}
						buildInfo={buildInfo}
						keyName={group.key.name}
						keyTags={group.key.tags}
						type={getGroupType(group.key)}
						provisioners={group.daemons}
					/>
				))}
			</Stack>
		</>
	);
};

// Ideally these would be generated and appear in typesGenerated.ts, but that is
// not currently the case. In the meantime, these are taken from verbatim from
// the corresponding codersdk declarations. The names remain unchanged to keep
// usage of these special values "grep-able".
// https://github.com/coder/coder/blob/7c77a3cc832fb35d9da4ca27df163c740f786137/codersdk/provisionerdaemons.go#L291-L295
const ProvisionerKeyIDBuiltIn = "00000000-0000-0000-0000-000000000001";
const ProvisionerKeyIDUserAuth = "00000000-0000-0000-0000-000000000002";
const ProvisionerKeyIDPSK = "00000000-0000-0000-0000-000000000003";

function getGroupType(key: ProvisionerKey) {
	switch (key.id) {
		case ProvisionerKeyIDBuiltIn:
			return "builtin";
		case ProvisionerKeyIDUserAuth:
			return "userAuth";
		case ProvisionerKeyIDPSK:
			return "psk";
		default:
			return "key";
	}
}
