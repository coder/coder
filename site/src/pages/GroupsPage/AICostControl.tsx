import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const SpendEstimateDocsLink: React.FC = () => (
	<Link
		href={docs("/ai-coder/ai-gateway/cost-controls#how-spend-is-calculated")}
		target="_blank"
		rel="noreferrer"
	>
		How spend is calculated
	</Link>
);
