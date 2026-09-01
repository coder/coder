import type { ComponentPropsWithRef, FC, PropsWithChildren } from "react";
import { Badge } from "./Badge";

export const EnabledBadge: FC = () => {
	return (
		<Badge className="option-enabled" variant="green">
			Enabled
		</Badge>
	);
};

export const DisabledBadge: FC<ComponentPropsWithRef<"div">> = ({
	...props
}) => {
	return (
		<Badge {...props} className="option-disabled">
			Disabled
		</Badge>
	);
};

export const EnterpriseBadge: FC = () => {
	return <Badge variant="purple">Enterprise</Badge>;
};

export const PremiumBadge: FC<PropsWithChildren> = ({
	children = "Premium",
}) => {
	return <Badge variant="magenta">{children}</Badge>;
};

export const AlphaBadge: FC = () => {
	return <Badge variant="purple">Alpha</Badge>;
};

export const DeprecatedBadge: FC = () => {
	return <Badge variant="warning">Deprecated</Badge>;
};

export const BadgeGroup: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex flex-row items-center gap-2 mb-4">{children}</div>
	);
};
