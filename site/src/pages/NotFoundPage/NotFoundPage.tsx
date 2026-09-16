import type { FC } from "react";

const NotFoundPage: FC = () => {
	return (
		<div className="w-full h-full flex flex-row justify-center items-center">
			<p className="flex gap-4">
				<span className="font-bold">404</span>
				This page could not be found.
			</p>
		</div>
	);
};

export default NotFoundPage;
