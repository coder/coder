import { type FC, type ReactNode, useState } from "react";

import type { ChatUsageLimitPeriod } from "#/api/typesGenerated";
import { isPositiveFiniteDollarAmount } from "#/utils/currency";

export interface DefaultLimitFormValues {
	enabled: boolean;
	period: ChatUsageLimitPeriod;
	amountDollars: string;
}

interface DefaultLimitControllerProps {
	initialValues: DefaultLimitFormValues;
	onSave: (values: DefaultLimitFormValues) => void;
	children: (props: {
		enabled: boolean;
		onEnabledChange: (enabled: boolean) => void;
		period: ChatUsageLimitPeriod;
		onPeriodChange: (period: ChatUsageLimitPeriod) => void;
		amountDollars: string;
		onAmountDollarsChange: (amount: string) => void;
		isAmountValid: boolean;
		saveDefault: () => void;
	}) => ReactNode;
}

export const DefaultLimitController: FC<DefaultLimitControllerProps> = ({
	initialValues,
	onSave,
	children,
}) => {
	const [enabled, setEnabled] = useState(initialValues.enabled);
	const [period, setPeriod] = useState<ChatUsageLimitPeriod>(
		initialValues.period,
	);
	const [amountDollars, setAmountDollars] = useState(
		initialValues.amountDollars,
	);
	const isAmountValid = !enabled || isPositiveFiniteDollarAmount(amountDollars);

	const handleSave = () => {
		if (enabled && !isPositiveFiniteDollarAmount(amountDollars)) {
			return;
		}

		onSave({ enabled, period, amountDollars });
	};

	return children({
		enabled,
		onEnabledChange: setEnabled,
		period,
		onPeriodChange: setPeriod,
		amountDollars,
		onAmountDollarsChange: setAmountDollars,
		isAmountValid,
		saveDefault: handleSave,
	});
};
