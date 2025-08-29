import { useTheme } from "@emotion/react";
import AlertTitle from "@mui/material/AlertTitle";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { useProxy } from "contexts/ProxyContext";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { docs } from "utils/docs";
import type { FileTree } from "utils/filetree";

interface WildcardHostnameWarningProps {
	fileTree?: FileTree;
}

export const WildcardHostnameWarning: FC<WildcardHostnameWarningProps> = ({
	fileTree,
}) => {
	const theme = useTheme();
	const { proxy } = useProxy();
	const { entitlements } = useDashboard();

	if (proxy.proxy?.wildcard_hostname) {
		return null;
	}

	if (fileTree) {
		const hasSubdomainCoderApp = Object.keys(fileTree).some((filePath) => {
			const content = fileTree[filePath];
			if (typeof content === "string") {
				return (
					content.includes('resource "coder_app"') &&
					/subdomain\s*=\s*true/.test(content)
				);
			}
			return false;
		});

		if (!hasSubdomainCoderApp) {
			return null;
		}
	}

	const isLicensed = entitlements.has_license;
	const message = isLicensed
		? "This template contains coder_app resources with subdomain = true, but your current region doesn't support subdomain applications. Users won't be able to access these applications until wildcard access URL is configured. Switch to a region with subdomain support or ask your administrator to enable it."
		: "This template contains coder_app resources with subdomain = true, but subdomain applications are not configured. Users won't be able to access these applications until you configure the --wildcard-access-url flag when starting the Coder server.";

	return (
		<Alert
			severity="warning"
			css={{
				borderRadius: 0,
				border: 0,
				borderBottom: `1px solid ${theme.palette.divider}`,
				borderLeft: `2px solid ${theme.palette.warning.main}`,
			}}
		>
			<AlertTitle>Workspace applications will not work</AlertTitle>
			<AlertDetail>
				<div>{message}</div>
				<div className="flex items-center gap-2 flex-wrap mt-2">
					<Link href={docs("/admin/setup#wildcard-access-url")} target="_blank">
						<span css={{ fontWeight: 600 }}>
							Learn more about wildcard access URL
						</span>
					</Link>
				</div>
			</AlertDetail>
		</Alert>
	);
};
