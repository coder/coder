export const TemplateBuilderTitle: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return <h2 className="mt-0 text-xl font-semibold mb-1">{children}</h2>;
};

export const TemplateBuilderSubtitle: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return (
		<p className="mt-0 text-sm font-normal text-content-secondary mb-4">
			{children}
		</p>
	);
};
