import type { FC } from "react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { NotificationBannerView } from "./NotificationBannerView";

export const NotificationBanners: FC = () => {
  const { appearance, entitlements } = useDashboard();
  const notificationBanners = appearance.notification_banners;

  const isEntitled =
    entitlements.features.appearance.entitlement !== "not_entitled";
  if (!isEntitled) {
    return null;
  }

  return (
    <>
      {notificationBanners
        .filter((banner) => banner.enabled)
        .map((banner) => (
          <NotificationBannerView
            key={banner.message}
            message={banner.message}
            backgroundColor={banner.background_color}
          />
        ))}
    </>
  );
};
