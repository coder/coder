import AlertTitle from "@mui/material/AlertTitle";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import type { FC } from "react";
import { docs } from "utils/docs";

export const WildcardHostnameWarning: FC = () => {
	return (
		<Alert severity="warning">
			<AlertTitle>Some workspace applications will not work</AlertTitle>
			<AlertDetail>
				<div>
					One or more apps in this workspace have{" "}
					<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
						subdomain = true
					</code>
					, which requires a Coder deployment with a Wildcard Access URL
					configured. Please contact your administrator.
				</div>
				<div className="pt-2">
					<Link href={docs("/admin/setup#wildcard-access-url")} target="_blank">
						Learn more about wildcard access URL
					</Link>
				</div>
			</AlertDetail>
		</Alert>
	);
};
