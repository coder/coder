import type { FC } from "react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { AnnouncementBannerView } from "./AnnouncementBannerView";

export const AnnouncementBanners: FC = () => {
  const { appearance, entitlements } = useDashboard();
  const announcementBanners = appearance.announcement_banners;

  const isEntitled =
    entitlements.features.appearance.entitlement !== "not_entitled";
  if (!isEntitled) {
    return null;
  }

  return (
    <>
      {announcementBanners
        .filter((banner) => banner.enabled)
        .map((banner) => (
          <AnnouncementBannerView
            key={banner.message}
            message={banner.message}
            backgroundColor={banner.background_color}
          />
        ))}
    </>
  );
};
