import Skeleton from "@mui/material/Skeleton";
import { visuallyHidden } from "@mui/utils";
import type * as TypesGen from "api/typesGenerated";
import { Abbr } from "components/Abbr/Abbr";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { displayError } from "components/GlobalSnackbar/utils";
import { Latency } from "components/Latency/Latency";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { useAuthenticated } from "hooks";
import { ChevronDownIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import { sortProxiesByLatency } from "./proxyUtils";

interface ProxyMenuProps {
	proxyContextValue: ProxyContextValue;
}

export const ProxyMenu: FC<ProxyMenuProps> = ({ proxyContextValue }) => {
	const [open, setOpen] = useState(false);
	const [refetchDate, setRefetchDate] = useState<Date>();
	const selectedProxy = proxyContextValue.proxy.proxy;
	const refreshLatencies = proxyContextValue.refetchProxyLatencies;
	const closeMenu = () => setOpen(false);
	const latencies = proxyContextValue.proxyLatencies;
	const isLoadingLatencies = Object.keys(latencies).length === 0;
	const isLoading = proxyContextValue.isLoading || isLoadingLatencies;
	const { permissions } = useAuthenticated();

	const proxyLatencyLoading = (proxy: TypesGen.Region): boolean => {
		if (!refetchDate) {
			// Only show loading if the user manually requested a refetch
			return false;
		}

		// Only show a loading spinner if:
		//  - A latency exists. This means the latency was fetched at some point, so
		//    the loader *should* be resolved.
		//  - The proxy is healthy. If it is not, the loader might never resolve.
		//  - The latency reported is older than the refetch date. This means the
		//    latency is stale and we should show a loading spinner until the new
		//    latency is fetched.
		const latency = latencies[proxy.id];
		return proxy.healthy && latency !== undefined && latency.at < refetchDate;
	};

	// This endpoint returns a 404 when not using enterprise.
	// If we don't return null, then it looks like this is
	// loading forever!
	if (proxyContextValue.error) {
		return null;
	}

	if (isLoading) {
		return (
			<Skeleton
				width="110px"
				height={40}
				css={{ borderRadius: 6, transform: "none" }}
			/>
		);
	}

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" size="lg">
					<span css={{ ...visuallyHidden }}>
						Latency for {selectedProxy?.display_name ?? "your region"}
					</span>

					{selectedProxy ? (
						<>
							<img
								// Empty alt text used because we don't want to double up on
								// screen reader announcements from visually-hidden span
								alt=""
								src={selectedProxy.icon_url}
							/>

							<Latency
								latency={latencies?.[selectedProxy.id]?.latencyMS}
								isLoading={proxyLatencyLoading(selectedProxy)}
							/>
						</>
					) : (
						"Select Proxy"
					)}

					<ChevronDownIcon className="text-content-primary !size-icon-lg" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="w-80">
				{proxyContextValue.proxies && proxyContextValue.proxies.length > 1 && (
					<DropdownMenuItem
						disabled
						className="flex flex-col gap-1 items-start data-[disabled]:opacity-100"
					>
						<div className="text-content-primary font-semibold text-left">
							Select a region nearest to you
						</div>
						<div className="text-xs text-content-secondary leading-relaxed">
							Workspace proxies improve terminal and web app connections to
							workspaces. This does not apply to{" "}
							<Abbr title="Command-Line Interface" pronunciation="initialism">
								CLI
							</Abbr>{" "}
							connections. A region must be manually selected, otherwise the
							default primary region will be used.
						</div>
					</DropdownMenuItem>
				)}

				{proxyContextValue.proxies && proxyContextValue.proxies.length > 1 && (
					<DropdownMenuSeparator />
				)}

				{proxyContextValue.proxies && (
					<DropdownMenuRadioGroup value={selectedProxy?.id}>
						{sortProxiesByLatency(proxyContextValue.proxies, latencies).map(
							(proxy) => (
								<DropdownMenuRadioItem
									value={proxy.id}
									key={proxy.id}
									onClick={(e) => {
										e.preventDefault();
										if (!proxy.healthy) {
											displayError("Please select a healthy workspace proxy.");
											closeMenu();
											return;
										}

										proxyContextValue.setProxy(proxy);
										closeMenu();
									}}
								>
									<div className="flex gap-3 items-center w-full">
										<div className="leading-none size-4">
											<img
												src={proxy.icon_url}
												alt=""
												className="object-contain size-full"
											/>
										</div>

										{proxy.display_name}

										<Latency
											className="ml-auto"
											latency={latencies?.[proxy.id]?.latencyMS}
											isLoading={proxyLatencyLoading(proxy)}
										/>
									</div>
								</DropdownMenuRadioItem>
							),
						)}
					</DropdownMenuRadioGroup>
				)}

				<DropdownMenuSeparator />

				{Boolean(permissions.editWorkspaceProxies) && (
					<DropdownMenuItem asChild>
						<Link to="/deployment/workspace-proxies">
							<span>Proxy settings</span>
						</Link>
					</DropdownMenuItem>
				)}

				<DropdownMenuItem
					onClick={(e) => {
						e.preventDefault();
						const refetchDate = refreshLatencies();
						setRefetchDate(refetchDate);
					}}
				>
					Refresh latencies
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
