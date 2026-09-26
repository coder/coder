import type { FC } from "react";
import { ItemsBadge } from "./ItemsBadge";
import { AIBridgeModelIcon } from "./icons/AIBridgeModelIcon";

type ModelsBadgeProps = {
	models: readonly string[];
};

export const ModelsBadge: FC<ModelsBadgeProps> = ({ models }) => (
	<ItemsBadge
		noun="models"
		items={models.map((model) => ({
			key: model,
			label: model,
			icon: <AIBridgeModelIcon model={model} className="size-icon-xs" />,
		}))}
	/>
);
