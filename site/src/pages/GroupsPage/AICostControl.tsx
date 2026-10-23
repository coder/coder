import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const SpendEstimateDocsLink: React.FC = () => (
	<Link
		href={docs(
			"/ai-governance/ai-gateway/cost-controls#how-spend-is-estimated",
		)}
		target="_blank"
		rel="noreferrer"
	>
		How spend is estimated
	</Link>
);
