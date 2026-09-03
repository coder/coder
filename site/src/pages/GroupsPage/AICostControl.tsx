import type { FC } from "react";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const SpendEstimateDocsLink: FC = () => (
	<Link
		href={docs("/ai-coder/ai-gateway/cost-controls#how-spend-is-estimated")}
		target="_blank"
		rel="noreferrer"
	>
		How spend is estimated
	</Link>
);
