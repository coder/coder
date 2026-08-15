import type { FC, SVGProps } from "react";
import { Spinner } from "#/components/Spinner/Spinner";

/**
 * Segmented spinner for chats in the `running` state. The sidebar
 * status icon can stay mounted for the lifetime of a long-running
 * chat, so it uses the shared stepped Spinner instead of a smooth
 * `animate-spin` icon that would keep the page rendering at the
 * display refresh rate.
 */
export const RunningSpinner: FC<SVGProps<SVGSVGElement>> = (props) => (
	<Spinner loading {...props} />
);
