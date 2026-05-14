import type { FC } from "react";

const AIAgentsGeneralPage: FC = () => {
	return (
		<div>
			<h1 className="text-3xl font-semibold mt-0 mb-2">
				Agent settings
			</h1>
			<p className="text-content-secondary text-sm">
				Configure defaults for delegated agents and other
				agent-specific capabilities.
			</p>
		</div>
	);
};

export default AIAgentsGeneralPage;
