import { useTheme } from "@emotion/react";
import AlertTitle from "@mui/material/AlertTitle";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { useProxy } from "contexts/ProxyContext";
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

	if (proxy.proxy?.wildcard_hostname) {
		return null;
	}

	if (fileTree) {
		// This regex matches any coder_app resource with the subdomain = true flag.
		const regex =
			/resource\s+"coder_app"\s+"[^"]+"\s*\{[\s\S]*?\bsubdomain\s*=\s*true\b[\s\S]*?\}/s;
		const hasSubdomainCoderApp = Object.keys(fileTree).some((filePath) => {
			const content = fileTree[filePath];
			return typeof content === "string" && regex.test(content);
		});

		if (!hasSubdomainCoderApp) {
			return null;
		}
	}

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
				<div>
					This template contains coder_app resources with subdomain = true, but
					subdomain applications are not configured. Users won't be able to
					access these applications until you configure the
					--wildcard-access-url flag when starting the Coder server.
				</div>
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
