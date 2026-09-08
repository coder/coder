import type { FC, PropsWithChildren } from "react";

export const TableToolbar: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="text-sm mb-2 mt-0 h-9 text-content-secondary flex items-center [&_strong]:text-content-primary">
			{children}
		</div>
	);
};
