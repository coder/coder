import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";

/**
 * Shown on premium page when any license is installed. The copy stays license-neutral
 * to also cover existing Enterprise licenses, which are not Premium.
 */
export const LicenseActivePanel: FC = () => {
	return (
		<div className="flex flex-col items-center gap-6 text-center max-w-md">
			<h1 className="m-0 font-semibold text-3xl text-content-primary">
				A license is already installed
			</h1>

			<p className="m-0 px-8 text-sm text-content-primary">
				This deployment already has a license.
				Review your entitlements on the Licenses page.
			</p>

			<Button asChild className="w-full">
				<RouterLink to="/deployment/licenses">View licenses</RouterLink>
			</Button>
		</div>
	);
};
