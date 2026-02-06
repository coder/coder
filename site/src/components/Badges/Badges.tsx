import { Badge } from "components/Badge/Badge";
import { Stack } from "components/Stack/Stack";
import {
	type FC,
	forwardRef,
	type HTMLAttributes,
	type PropsWithChildren,
} from "react";

export const EnabledBadge: FC = () => {
	return (
		<Badge className="option-enabled" variant="green" border="solid">
			Enabled
		</Badge>
	);
};

export const EntitledBadge: FC = () => {
	return (
		<Badge border="solid" variant="green">
			Entitled
		</Badge>
	);
};
export const DisabledBadge: FC = forwardRef<
	HTMLDivElement,
	HTMLAttributes<HTMLDivElement>
>((props, ref) => {
	return (
		<Badge ref={ref} {...props} className="option-disabled">
			Disabled
		</Badge>
	);
});

export const EnterpriseBadge: FC = () => {
	return (
		<Badge variant="info" border="solid">
			Enterprise
		</Badge>
	);
};

interface PremiumBadgeProps {
	children?: React.ReactNode;
}

export const PremiumBadge: FC<PremiumBadgeProps> = ({
	children = "Premium",
}) => {
	return (
		<Badge variant="purple" border="solid">
			{children}
		</Badge>
	);
};

export const PreviewBadge: FC = () => {
	return (
		<Badge variant="purple" border="solid">
			Preview
		</Badge>
	);
};

export const AlphaBadge: FC = () => {
	return (
		<Badge variant="purple" border="solid">
			Alpha
		</Badge>
	);
};

export const DeprecatedBadge: FC = () => {
	return (
		<Badge variant="warning" border="solid">
			Deprecated
		</Badge>
	);
};

export const Badges: FC<PropsWithChildren> = ({ children }) => {
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
