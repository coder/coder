import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const SpendEstimateDocsLink: React.FC = () => (
	<Link
		size="sm"
		href={docs(
			"/ai-governance/ai-gateway/cost-controls#how-spend-is-calculated",
		)}
		target="_blank"
		rel="noreferrer"
	>
		How spend is calculated
	</Link>
);
