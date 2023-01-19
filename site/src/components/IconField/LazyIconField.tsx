import { lazy, FC, Suspense } from "react"
import { IconFieldProps } from "./types"

const IconField = lazy(() => import("./IconField"))

export const LazyIconField: FC<IconFieldProps> = (props) => {
  return (
    <Suspense>
      <IconField {...props} />
    </Suspense>
  )
}
