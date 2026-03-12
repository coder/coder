import { useTheme } from "@emotion/react";
import type {
	HealthcheckReport,
	HealthSeverity,
	NetcheckReport,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { Table, TableBody, TableCell, TableRow } from "components/Table/Table";
import { MapPinIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useOutletContext } from "react-router";
import { pageTitle } from "utils/page";
import {
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Logs,
	Main,
	SectionLabel,
	StatusIcon,
} from "./Content";
import { DismissWarningButton } from "./DismissWarningButton";
import { healthyColor } from "./healthyColor";

type BooleanKeys<T> = {
	[K in keyof T]: T[K] extends boolean | null ? K : never;
}[keyof T];

interface FlagInfo {
	label: string;
	description: string;
	invert?: boolean;
}

const flagDescriptions: Record<BooleanKeys<NetcheckReport>, FlagInfo> = {
	UDP: {
		label: "UDP",
		description: "Whether a UDP STUN round trip completed successfully.",
	},
	IPv6: {
		label: "IPv6",
		description: "Whether an IPv6 STUN round trip completed successfully.",
	},
	IPv4: {
		label: "IPv4",
		description: "Whether an IPv4 STUN round trip completed successfully.",
	},
	IPv6CanSend: {
		label: "IPv6 Send",
		description: "Whether this server can send IPv6 packets.",
	},
	IPv4CanSend: {
		label: "IPv4 Send",
		description: "Whether this server can send IPv4 packets.",
	},
	OSHasIPv6: {
		label: "OS IPv6 Support",
		description: "Whether the operating system supports IPv6.",
	},
	ICMPv4: {
		label: "ICMP Ping",
		description: "Whether an ICMPv4 round trip completed successfully.",
	},
	MappingVariesByDestIP: {
		label: "No Symmetric NAT",
		description:
			"Whether STUN results are consistent across destinations. Symmetric NAT may degrade peer-to-peer connectivity.",
		invert: true,
	},
	HairPinning: {
		label: "NAT Hairpinning",
		description:
			"Whether the router supports communication between local devices through the public IP address.",
	},
	UPnP: {
		label: "UPnP",
		description: "Whether Universal Plug and Play was detected on the LAN.",
	},
	PMP: {
		label: "NAT-PMP",
		description: "Whether NAT Port Mapping Protocol was detected on the LAN.",
	},
	PCP: {
		label: "PCP",
		description: "Whether Port Control Protocol was detected on the LAN.",
	},
	CaptivePortal: {
		label: "No Captive Portal",
		description:
			"Whether HTTP traffic is free from captive portal interception.",
		invert: true,
	},
};

interface FlagGroup {
	title: string;
	flags: BooleanKeys<NetcheckReport>[];
}

const flagGroups: FlagGroup[] = [
	{
		title: "Connectivity",
		flags: ["UDP", "IPv4", "IPv6", "ICMPv4", "CaptivePortal"],
	},
	{
		title: "IPv6 Support",
		flags: ["OSHasIPv6", "IPv4CanSend", "IPv6CanSend"],
	},
	{
		title: "NAT Traversal",
		flags: ["MappingVariesByDestIP", "HairPinning"],
	},
	{
		title: "Port Mapping",
		flags: ["UPnP", "PMP", "PCP"],
	},
];

const DERPPage: FC = () => {
	const { derp } = useOutletContext<HealthcheckReport>();
	const { netcheck, regions, netcheck_logs: logs } = derp;
	const safeNetcheck = netcheck || ({} as NetcheckReport);
	const theme = useTheme();

	return (
		<>
			<title>{pageTitle("DERP - Health")}</title>

			<Header>
				<HeaderTitle>
					<HealthyDot severity={derp.severity as HealthSeverity} />
					DERP
				</HeaderTitle>
				<DismissWarningButton healthcheck="DERP" />
			</Header>

			<Main>
				{derp.warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
							prominent
						>
							{warning.message}
						</Alert>
					);
				})}

				<section>
					<SectionLabel>Network Checks</SectionLabel>
					{flagGroups.map((group) => (
						<div key={group.title} className="mb-6">
							<h5 className="text-xs uppercase tracking-wide text-content-secondary m-0 mb-2">
								{group.title}
							</h5>
							<Table>
								<TableBody>
									{group.flags.map((flag) => (
										<TableRow key={flag}>
											<TableCell className="w-8">
												<StatusIcon
													value={
														safeNetcheck[flag] === null
															? null
															: flagDescriptions[flag].invert
																? !safeNetcheck[flag]
																: safeNetcheck[flag]
													}
												/>
											</TableCell>
											<TableCell className="font-medium whitespace-nowrap w-36">
												{flagDescriptions[flag].label}
											</TableCell>
											<TableCell className="text-content-secondary">
												{flagDescriptions[flag].description}
											</TableCell>
										</TableRow>
									))}
								</TableBody>
							</Table>
						</div>
					))}
				</section>

				<section>
					<SectionLabel>Regions</SectionLabel>
					<div className="flex flex-wrap gap-3">
						{Object.values(regions ?? {})
							.filter((region) => {
								// Values can technically be null
								return region !== null;
							})
							.sort((a, b) => {
								if (a.region && b.region) {
									return a.region.RegionName.localeCompare(b.region.RegionName);
								}
								return 0;
							})
							.map(({ severity, region }) => {
								if (!region) {
									return null;
								}
								return (
									<Button variant="outline" key={region.RegionID} asChild>
										<Link to={`/health/derp/regions/${region.RegionID}`}>
											<MapPinIcon
												style={{
													color: healthyColor(
														theme,
														severity as HealthSeverity,
													),
												}}
											/>
											{region.RegionName}
										</Link>
									</Button>
								);
							})}
					</div>
				</section>
				<section>
					<SectionLabel>Logs</SectionLabel>
					<Logs
						lines={logs}
						className="rounded-lg border border-solid border-border text-content-secondary"
					/>
				</section>
			</Main>
		</>
	);
};

export default DERPPage;
