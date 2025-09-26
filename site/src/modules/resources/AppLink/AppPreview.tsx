import { Stack } from "components/Stack/Stack";
import type { FC, PropsWithChildren } from "react";
export const AppPreview: FC<PropsWithChildren> = ({ children }) => {
	return (
		<Stack
			className="flex items-center h-8 px-3 rounded-full border border-solid border-surface-quaternary text-content-primary bg-surface-secondary flex-shrink-0 w-fit text-xs [&>svg]:w-[13px] [&>img]:w-[13px]"
			alignItems="center"
			direction="row"
			spacing={1}
		>
			{children}
		</Stack>
	);
};
