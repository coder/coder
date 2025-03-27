import { API } from "api/api";
import { buildInfo } from "api/queries/buildInfo";
import { experiments } from "api/queries/experiments";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useEffect, useState } from "react";
import { useQuery } from "react-query";

interface WebpushNotifications {
	readonly enabled: boolean;
	readonly subscribed: boolean;
	readonly loading: boolean;

	subscribe(): Promise<void>;
	unsubscribe(): Promise<void>;
}

export const useWebpushNotifications = (): WebpushNotifications => {
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const enabledExperimentsQuery = useQuery(experiments(metadata.experiments));

	const [subscribed, setSubscribed] = useState<boolean>(false);
	const [loading, setLoading] = useState<boolean>(true);
	const [enabled, setEnabled] = useState<boolean>(false);

	useEffect(() => {
		// Check if the experiment is enabled.
		if (enabledExperimentsQuery.data?.includes("web-push")) {
			setEnabled(true);
		}

		// Check if browser supports push notifications
		if (!("Notification" in window) || !("serviceWorker" in navigator)) {
			setSubscribed(false);
			setLoading(false);
			return;
		}

		const checkSubscription = async () => {
			try {
				const registration = await navigator.serviceWorker.ready;
				const subscription = await registration.pushManager.getSubscription();
				setSubscribed(!!subscription);
			} catch (error) {
				console.error("Error checking push subscription:", error);
				setSubscribed(false);
			} finally {
				setLoading(false);
			}
		};

		checkSubscription();
	}, [enabledExperimentsQuery.data]);

	const subscribe = async (): Promise<void> => {
		try {
			setLoading(true);
			const registration = await navigator.serviceWorker.ready;
			const vapidPublicKey = buildInfoQuery.data?.webpush_public_key;

			const subscription = await registration.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: vapidPublicKey,
			});
			const json = subscription.toJSON();
			if (!json.keys || !json.endpoint) {
				throw new Error("No keys or endpoint found");
			}

			await API.createWebPushSubscription("me", {
				endpoint: json.endpoint,
				auth_key: json.keys.auth,
				p256dh_key: json.keys.p256dh,
			});

			setSubscribed(true);
		} catch (error) {
			console.error("Subscription failed:", error);
			throw error;
		} finally {
			setLoading(false);
		}
	};

	const unsubscribe = async (): Promise<void> => {
		try {
			setLoading(true);
			const registration = await navigator.serviceWorker.ready;
			const subscription = await registration.pushManager.getSubscription();

			if (subscription) {
				await subscription.unsubscribe();
				setSubscribed(false);
			}
		} catch (error) {
			console.error("Unsubscription failed:", error);
			throw error;
		} finally {
			setLoading(false);
		}
	};

	return {
		subscribed,
		enabled,
		loading: loading || buildInfoQuery.isLoading,
		subscribe,
		unsubscribe,
	};
};
