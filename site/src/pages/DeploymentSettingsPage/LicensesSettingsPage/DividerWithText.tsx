import type { FC, PropsWithChildren } from "react";

export const DividerWithText: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex items-center">
			<div className="border-0 border-b-2 border-solid border-border w-full" />
			<span className="py-1 px-4 text-xl text-content-secondary">
				{children}
			</span>
			<div className="border-0 border-b-2 border-solid border-border w-full" />
		</div>
	);
};
