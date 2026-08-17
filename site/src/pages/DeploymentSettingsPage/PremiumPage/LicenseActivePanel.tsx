import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";

/**
 * Shown on premium page when any license is installed. The copy stays license-neutral
 * to also cover existing Enterprise licenses, which are not Premium.
 */
export const LicenseActivePanel: FC = () => {
	return (
		<div className="flex flex-col gap-6 items-start">
			<div className="flex flex-col gap-2">
				<h1 className="m-0 font-semibold text-2xl text-content-primary">
					A license is already installed
				</h1>
				<p className="m-0 text-sm text-content-secondary">
					This deployment already has a license, so a trial is not available.
					Review your entitlements on the Licenses page.
				</p>
			</div>

			<Button asChild>
				<RouterLink to="/deployment/licenses">View licenses</RouterLink>
			</Button>
		</div>
	);
};
