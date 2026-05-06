import type { FC } from "react";
import { Link } from "react-router";
import { CoderIcon } from "../../components/Icons/CoderIcon";
import { pageTitle } from "../../utils/page";
import { LunarLander } from "./LunarLander";

const CoderCupPage: FC = () => {
	return (
		<div className="relative w-screen h-screen bg-black overflow-hidden">
			<title>{pageTitle("Codernauts")}</title>

			{/* Coder logo - links back to the main app */}
			<Link
				to="/workspaces"
				className="absolute top-3 left-3 z-10 opacity-60 hover:opacity-100 transition-opacity"
			>
				<CoderIcon className="!w-8 !h-4 text-white" />
			</Link>

			<LunarLander />
		</div>
	);
};

export default CoderCupPage;
