import { Badge } from "components/Badge/Badge";
import { Stack } from "components/Stack/Stack";

export const EnabledBadge: React.FC = () => {
	return (
		<Badge className="option-enabled" variant="green" border="solid">
			Enabled
		</Badge>
	);
};

export const EntitledBadge: React.FC = () => {
	return (
		<Badge border="solid" variant="green">
			Entitled
		</Badge>
	);
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
	return (
		<Badge variant="info" border="solid">
			Enterprise
		</Badge>
	);
};

interface PremiumBadgeProps {
	children?: React.ReactNode;
}

export const PremiumBadge: React.FC<PremiumBadgeProps> = ({
	children = "Premium",
}) => {
	return (
		<Badge variant="purple" border="solid">
			{children}
		</Badge>
	);
};

export const PreviewBadge: React.FC = () => {
	return (
		<Badge variant="purple" border="solid">
			Preview
		</Badge>
	);
};

export const AlphaBadge: React.FC = () => {
	return (
		<Badge variant="purple" border="solid">
			Alpha
		</Badge>
	);
};

export const DeprecatedBadge: React.FC = () => {
	return (
		<Badge variant="warning" border="solid">
			Deprecated
		</Badge>
	);
};

export const Badges: React.FC<React.PropsWithChildren> = ({ children }) => {
	return (
		<Stack
			css={{ margin: "0 0 16px" }}
			direction="row"
			alignItems="center"
			spacing={1}
		>
			{children}
		</Stack>
	);
};
