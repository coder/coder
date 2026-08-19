import type { FC } from "react";
import { CODER_PRIVACY_POLICY_LINK } from "#/modules/licenses/trialLicense";

export const PrivacyPolicyNotice: FC = () => {
	return (
		<>
			The information you provide will be treated in accordance with the{" "}
			<a
				href={CODER_PRIVACY_POLICY_LINK}
				target="_blank"
				rel="noreferrer"
				className="text-content-link hover:underline"
			>
				Coder Privacy Policy
			</a>
			.
		</>
	);
};
