import type { FC } from "react";

const SecretsPage: FC = () => {
	return (
		<div>
			<h1 className="text-3xl font-semibold mt-0 mb-2">Secrets</h1>
			<p className="text-content-secondary text-sm">
				Manage deployment secrets and credentials.
			</p>
		</div>
	);
};

export default SecretsPage;
