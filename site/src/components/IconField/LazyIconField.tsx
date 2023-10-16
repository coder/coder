import { lazy, Suspense, type ComponentProps } from "react";

const IconField = lazy(() => import("./IconField"));

export const LazyIconField = (props: ComponentProps<typeof IconField>) => {
  return (
    <Suspense fallback={<div role="progressbar" data-testid="loader" />}>
      <IconField {...props} />
    </Suspense>
  );
};
