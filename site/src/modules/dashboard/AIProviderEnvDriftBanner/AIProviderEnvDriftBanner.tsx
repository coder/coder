import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "#/components/Link/Link";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { docs } from "#/utils/docs";

interface AIProviderEnvDriftBannerViewProps {
	docsHref: string;
}

/**
 * AIProviderEnvDriftBannerView is the presentational banner. It is pure
 * (props only) so it can be exercised in Storybook without dashboard
 * context. Styling mirrors the single-message LicenseBannerView so the
 * two banners stack consistently.
 */
export const AIProviderEnvDriftBannerView: FC<
	AIProviderEnvDriftBannerViewProps
> = ({ docsHref }) => {
	return (
		<div role="status" className="flex items-center p-3 bg-surface-secondary">
			<div className="flex min-w-0 flex-1 items-start gap-2">
				<div className="flex h-6 items-center">
					<TriangleAlertIcon className="size-4 text-content-warning" />
				</div>
				<div className="flex min-h-6 min-w-0 flex-1 items-center text-xs leading-4 text-content-primary">
					<span>
						Changes to the deprecated AI provider environment variables
						(CODER_AIBRIDGE_*) are ineffective. Manage AI providers through the
						dashboard, which is the source of truth.{" "}
						<Link
							className="text-xs font-medium !text-content-link"
							href={docsHref}
							target="_blank"
						>
							View setup docs
						</Link>
					</span>
				</div>
			</div>
		</div>
	);
};

/**
 * AIProviderEnvDriftBanner warns admins when deprecated CODER_AIBRIDGE_*
 * env configuration drifts from the AI provider rows in the database,
 * which makes those env changes ineffective. It renders nothing when no
 * drift was detected at startup.
 */
export const AIProviderEnvDriftBanner: FC = () => {
	const { appearance } = useDashboard();

	if (!appearance.ai_providers_env_drift_detected) {
		return null;
	}

	return (
		<AIProviderEnvDriftBannerView
			docsHref={docs("/ai-coder/ai-gateway/setup")}
		/>
	);
};
