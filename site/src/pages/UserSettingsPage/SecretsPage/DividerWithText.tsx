import type { FC, PropsWithChildren } from "react";

// DividerWithText renders a horizontal rule with a centered label. It is a
// dialog-sized variant of the shared license divider, kept local to the
// secrets feature so it can use compact typography without affecting other
// consumers.
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
