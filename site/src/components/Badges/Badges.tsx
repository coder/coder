import { Badge } from "#/components/Badge/Badge";
import { Stack } from "#/components/Stack/Stack";

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
		<Stack className="mb-4" direction="row" alignItems="center" spacing={1}>
			{children}
		</Stack>
	);
};
