import type { Meta, StoryObj } from "@storybook/react-vite";
import { PremiumBadge } from "components/Badges/Badges";
import {
	Paywall,
	PaywallContent,
	PaywallCTA,
	PaywallDescription,
	PaywallFeature,
	PaywallFeatures,
	PaywallHeading,
	PaywallSeparator,
	PaywallStack,
	PaywallTitle,
} from "./Paywall";

const meta: Meta<typeof Paywall> = {
	title: "components/Paywall",
	component: Paywall,
};

export default meta;
type Story = StoryObj<typeof Paywall>;

export const Default: Story = {
	args: {
		children: (
			<>
				<PaywallContent>
					<PaywallHeading>
						<PaywallTitle>Black Lotus</PaywallTitle>
						<PremiumBadge />
					</PaywallHeading>
					<PaywallDescription>
						Adds 3 mana of any single color of your choice to your mana pool,
						then is discarded. Tapping this artifact can be played as an
						interrupt.
					</PaywallDescription>
				</PaywallContent>
				<PaywallSeparator />
				<PaywallStack>
					<PaywallFeatures>
						<PaywallFeature>
							High availability & workspace proxies
						</PaywallFeature>
						<PaywallFeature>
							Multi-org & role-based access control
						</PaywallFeature>
						<PaywallFeature>24x7 global support with SLA</PaywallFeature>
						<PaywallFeature>
							Unlimited Git & external auth integrations
						</PaywallFeature>
					</PaywallFeatures>
					<PaywallCTA href="https://coder.com/pricing#compare-plans">
						Learn about Premium
					</PaywallCTA>
				</PaywallStack>
			</>
		),
	},
};
