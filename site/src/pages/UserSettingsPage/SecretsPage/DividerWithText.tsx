import type { FC, PropsWithChildren } from "react";

export const DividerWithText: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex items-center">
			<div className="w-full border-0 border-b border-solid border-border" />
			<span className="whitespace-nowrap px-3 text-xs text-content-secondary">
				{children}
			</span>
			<div className="w-full border-0 border-b border-solid border-border" />
		</div>
	);
};
