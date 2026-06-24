import type { ComponentProps, FC } from "react";

export const TemplateBuilderTitle: FC<ComponentProps<"h2">> = ({
	children,
}) => {
	return <h2 className="text-xl font-semibold mb-1">{children}</h2>;
};

export const TemplateBuilderSubtitle: FC<ComponentProps<"p">> = ({
	children,
}) => {
	return (
		<p className="mt-0 text-sm font-normal text-content-secondary mb-4">
			{children}
		</p>
	);
};
