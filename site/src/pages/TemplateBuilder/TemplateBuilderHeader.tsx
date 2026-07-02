import type { FC, PropsWithChildren } from "react";

export const TemplateBuilderTitle: FC<PropsWithChildren> = ({ children }) => {
	return <h2 className="mt-0 text-2xl font-semibold mb-1">{children}</h2>;
};

export const TemplateBuilderSubtitle: FC<PropsWithChildren> = ({
	children,
}) => {
	return (
		<p className="mt-0 text-sm font-normal text-content-secondary mb-4">
			{children}
		</p>
	);
};
