import { Badge } from "#/components/Badge/Badge";

export const EnabledBadge: React.FC = () => {
	return (
		<Badge className="option-enabled" variant="green">
			Enabled
		</Badge>
	);
};

export const EntitledBadge: React.FC = () => {
	return <Badge variant="green">Entitled</Badge>;
};

export const DisabledBadge: React.FC<React.ComponentPropsWithRef<"div">> = ({
	...props
}) => {
	return (
		<Badge {...props} className="option-disabled">
			Disabled
		</Badge>
	);
};

export const EnterpriseBadge: React.FC = () => {
	return <Badge variant="purple">Enterprise</Badge>;
};

interface PremiumBadgeProps {
	children?: React.ReactNode;
}

export const PremiumBadge: React.FC<PremiumBadgeProps> = ({
	children = "Premium",
}) => {
	return <Badge variant="magenta">{children}</Badge>;
};

export const PreviewBadge: React.FC = () => {
	return <Badge variant="purple">Preview</Badge>;
};

export const AlphaBadge: React.FC = () => {
	return <Badge variant="purple">Alpha</Badge>;
};

export const DeprecatedBadge: React.FC = () => {
	return <Badge variant="warning">Deprecated</Badge>;
};

export const Badges: React.FC<React.PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex flex-row items-center gap-2 mb-4">{children}</div>
	);
};
