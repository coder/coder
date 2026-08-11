import { useQuery } from "react-query";
import { deploymentConfig } from "#/api/queries/deployment";
import { useAuthenticated } from "#/hooks/useAuthenticated";

/**
 * Whether the template builder is available. It is on by default and only
 * disabled on airgapped deployments via CODER_DISABLE_TEMPLATE_BUILDER,
 * which makes all /api/v2/templatebuilder/* endpoints return 404.
 */
export const useTemplateBuilderEnabled = (): boolean => {
	const { permissions } = useAuthenticated();
	const deploymentConfigQuery = useQuery({
		...deploymentConfig(),
		enabled: permissions.createTemplates,
	});
	return (
		deploymentConfigQuery.isSuccess &&
		!deploymentConfigQuery.data?.config?.template_builder?.disabled
	);
};
